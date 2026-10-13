package horizon

import (
	"context"
	"encoding/json"
	"fmt"

	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/task"
)

// Execution-local capability selection and correlation state.
type capabilityExecution struct {
	runner      task.CapabilityRunner
	definitions []capabilities.Definition
	advertised  map[capabilities.Name]struct{}
	seen        map[string]struct{}
	maxTurns    uint16
}

// PrepareCapabilities snapshots selection and budget, resolves definitions once,
// and validates existing history before the first inference or executor side effect.
func (p *process) PrepareCapabilities(ctx context.Context) (*capabilityExecution, error) {
	// Snapshot task selection and resolve the effective turn budget.
	selected := slices.Clone(p.task.Capabilities)
	maxTurns := p.config.MaxToolTurns
	if parameters := p.task.Model.Parameters; parameters != nil {
		configured, exists, err := parameters.MaxToolTurns()
		if err != nil {
			return nil, err
		}
		if exists {
			maxTurns = configured
		}
	}
	if maxTurns == 0 {
		return nil, fmt.Errorf("selected capabilities require a positive max_tool_turns allowance")
	}

	// Require a host executor and validate selection before asking it for definitions.
	runner, ok := p.runner.(task.CapabilityRunner)
	if !ok {
		return nil, errors.ErrCapabilityProviderRequired
	}

	execution := &capabilityExecution{
		runner:     runner,
		advertised: make(map[capabilities.Name]struct{}, len(selected)),
		seen:       make(map[string]struct{}),
		maxTurns:   maxTurns,
	}
	for _, name := range selected {
		if err := name.Validate(); err != nil {
			return nil, err
		}
		if _, exists := execution.advertised[name]; exists {
			return nil, fmt.Errorf("duplicate selected capability %q", name)
		}
		execution.advertised[name] = struct{}{}
	}

	// Resolve once, then require an exact, valid definition for every selection.
	var definitions []capabilities.Definition
	if err := cancelAndTrace(ctx, "capabilities.lookup", func(ctx context.Context) error {
		var err error
		definitions, err = runner.LookupCapabilities(ctx, slices.Clone(selected))
		return err
	}); err != nil {
		return nil, fmt.Errorf("lookup capabilities: %w", err)
	}
	byName := make(map[capabilities.Name]capabilities.Definition, len(definitions))
	for _, definition := range definitions {
		if err := definition.Name.Validate(); err != nil {
			return nil, err
		}
		if _, selected := execution.advertised[definition.Name]; !selected {
			return nil, fmt.Errorf("unselected capability definition %q", definition.Name)
		}
		if _, exists := byName[definition.Name]; exists {
			return nil, fmt.Errorf("duplicate capability definition %q", definition.Name)
		}
		object, err := definition.InputSchema.Object()
		if err != nil {
			return nil, fmt.Errorf("capability %q schema: %w", definition.Name, err)
		}
		if object["type"] != "object" {
			return nil, fmt.Errorf("capability %q: %w", definition.Name, errors.ErrInvalidJSONSchema)
		}
		byName[definition.Name] = definition
	}
	// Keep selection order and own copies of the host-provided definitions.
	for _, name := range selected {
		definition, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("capability %q: %w", name, capabilities.ErrNotFound)
		}
		execution.definitions = append(execution.definitions, definition.Clone())
	}

	p.output.Capabilities = selected

	// Custom renderers may provide earlier calls. Reserve those IDs before starting
	// so the loop cannot collide with history or execute a malformed transcript.
	priorCalls, err := capabilityCalls(p.messages)
	if err != nil {
		return nil, err
	}
	if err := execution.validateCallIDs(priorCalls); err != nil {
		return nil, err
	}
	return execution, nil
}

// CapabilitiesLoop coordinates guarded inference and sequential call batches.
// Rendering, provider construction, and definition lookup are not repeated.
func (p *process) CapabilitiesLoop(ctx context.Context) error {
	// Prepare the fixed capability selection and advertise it on every turn.
	execution, err := p.PrepareCapabilities(ctx)
	if err != nil {
		return err
	}
	p.request.Tools = execution.definitions

	var turns uint16
	for {
		// Check cancellation each turn.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Run one inference turn and extract tool calls.
		response, generationErr := p.Generate(ctx)
		if response == nil {
			return generationErr
		}
		calls, callErr := capabilityCalls(response.Output)

		// An inference error ends the loop; captureResponse keeps only eligible
		// final content and preserves the inference error.
		if generationErr != nil {
			return p.captureResponse(response, generationErr)
		}
		if callErr != nil {
			return callErr
		}

		// If there are no further calls, return the result.
		if len(calls) == 0 {
			return p.captureResponse(response, nil)
		}

		// Ensure we don't exceed the maximum number of tool turns.
		if turns >= execution.maxTurns {
			return errors.ErrMaxToolTurns
		}
		turns++

		// Validate the batch of call IDs before executing any.
		if err := execution.validateCallIDs(calls); err != nil {
			return err
		}

		// Resolve calls sequentially in provider order before requesting continuation.
		for _, call := range calls {
			if err := p.executeCapability(ctx, execution, call); err != nil {
				return err
			}
		}
	}
}

// Extracts tool calls from the messages with no mutations.
func capabilityCalls(messages prompts.Prompts) ([]prompts.ToolCall, error) {
	var calls []prompts.ToolCall
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Type == prompts.TypeToolCall && len(message.ToolCalls) == 0 {
			return nil, capabilities.ErrUnsupportedResponse
		}
		if len(message.ToolCalls) > 0 && message.Role != prompts.RoleAssistant {
			return nil, capabilities.ErrUnsupportedResponse
		}
		calls = append(calls, message.ToolCalls...)
	}
	return calls, nil
}

// Ensures call IDs are present and cannot be reused.
func (e *capabilityExecution) validateCallIDs(calls []prompts.ToolCall) error {
	for _, call := range calls {
		if call.CallID == "" {
			return errors.ErrToolCallIDRequired
		}
		if _, exists := e.seen[call.CallID]; exists {
			return fmt.Errorf("duplicate call ID: %w", capabilities.ErrInvalidToolPrompt)
		}
		e.seen[call.CallID] = struct{}{}
	}
	return nil
}

// executeCapability runs one validated call and appends its id correlated
// result. Retryable failures become model-visible corrections; terminal
// failures stop the loop.
func (p *process) executeCapability(ctx context.Context, execution *capabilityExecution, call prompts.ToolCall) (err error) {
	ctx, span := tracer.Start(ctx, "capabilities.execute")
	span.SetAttributes(
		attribute.String("capabilities.name", call.Name),
		attribute.String("capabilities.call_id", call.CallID),
	)
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, "capability failed")
		}
		span.End()
	}()

	// Do not dispatch a call after cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Reject unadvertised names, allowing the model to fix.
	name := capabilities.Name(call.Name)
	if _, ok := execution.advertised[name]; !ok {
		span.AddEvent("capabilities.invalid_capability", trace.WithAttributes(attribute.String("capabilities_name", call.Name)))
		p.messages = append(p.messages, &prompts.Prompt{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  call.CallID,
				Content: "The requested capability is unavailable. Choose an advertised capability.",
			}},
		})
		return nil
	}

	// Reject malformed arguments, allowing the model to fix.
	var arguments map[string]any
	argumentErr := json.Unmarshal([]byte(call.Arguments), &arguments)
	if argumentErr == nil && arguments == nil {
		argumentErr = errors.ErrToolCallInvalid
	}
	if argumentErr != nil {
		span.AddEvent("capabilities.invalid_arguments", trace.WithAttributes(
			attribute.String("capabilities_name", call.Name),
			attribute.String("arguments_error", argumentErr.Error()),
		))
		p.messages = append(p.messages, &prompts.Prompt{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  call.CallID,
				Content: "Arguments must be one valid JSON object. Correct them and try again.",
			}},
		})
		return nil
	}

	// Count and dispatch only calls that passed local validation.
	p.output.Usage.ToolCalls++
	response, executionErr := execution.runner.ExecuteCapability(ctx, capabilities.Request{
		CallID:    call.CallID,
		Name:      name,
		Arguments: arguments,
	})

	// Check for cancellation after long-running tool calls.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Retryable failures let the model correct its next request; terminal
	// errors stop.
	if executionErr != nil {
		if !executionErr.Retryable {
			return fmt.Errorf("execute capability %q: %w", name, executionErr)
		}
		retryAttributes := []attribute.KeyValue{
			attribute.String("message", executionErr.Message),
		}
		if executionErr.Cause != nil {
			retryAttributes = append(retryAttributes,
				attribute.String("error", executionErr.Cause.Error()),
				attribute.String("error.type", fmt.Sprintf("%T", executionErr.Cause)),
			)
		}
		span.AddEvent("capabilities.retryable_failure", trace.WithAttributes(retryAttributes...))
		p.messages = append(p.messages, &prompts.Prompt{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  call.CallID,
				Content: executionErr.Message,
			}},
		})
		return nil
	}

	// Convert the contribution using the originating call ID. Unsupported kinds
	// become a safe result so the model can decide how to continue.
	messages, err := response.Process(call.CallID)
	if err != nil {
		p.messages = append(p.messages, &prompts.Prompt{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  call.CallID,
				Content: "The capability returned an unsupported result.",
			}},
		})
		return nil
	}

	p.messages = append(p.messages, messages...)
	return nil
}
