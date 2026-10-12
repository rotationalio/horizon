package horizon

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/x/mime"
)

// Generate performs one guarded inference turn, accounting usage and appending
// accepted response messages.
func (p *process) Generate(ctx context.Context) (*provider.Response, error) {
	// The process messages are the authoritative transcript, including reasoning.
	p.request.Input = p.messages

	// Input guards validate a snapshot so that mutations are ignored.
	if guard, ok := p.runner.(task.InputGuard); ok {
		if err := cancelAndTrace(ctx, "guard.input", func(context.Context) error {
			return guard.ProtectInput(p.request.Clone())
		}); err != nil {
			return nil, fmt.Errorf("protect inference input: %w", err)
		}
	}

	// Perform generation inside its own span.
	requestCtx, span := tracer.Start(ctx, "provider.generate")
	span.SetAttributes(attribute.String("provider.model", p.request.Model))

	p.output.Usage.Invocations++
	response, generateErr := p.provider.Generate(requestCtx, p.request)

	responseAttributes := []attribute.KeyValue{attribute.Bool("provider.response_present", response != nil)}
	if response != nil {
		responseAttributes = append(responseAttributes,
			attribute.String("provider.response_model", response.Model),
			attribute.Int64("provider.input_tokens", response.Usage.InputTokens),
			attribute.Int64("provider.output_tokens", response.Usage.OutputTokens),
			attribute.Int64("provider.total_tokens", response.Usage.TotalTokens),
		)
	}
	span.AddEvent("provider.response", trace.WithAttributes(responseAttributes...))
	if generateErr != nil {
		span.SetStatus(codes.Error, "provider request failed")
	}
	span.End()

	// A returned response means the provider call completed; keep its accounting
	// even when the caller canceled before Horizon could deliver the content.
	if response != nil {
		p.output.Usage.InputTokens += response.Usage.InputTokens
		p.output.Usage.OutputTokens += response.Usage.OutputTokens
		p.output.Usage.TotalTokens += response.Usage.TotalTokens
		p.output.Usage.APICost += response.Usage.APICost
	}

	if ctx.Err() != nil {
		return nil, errors.Join(generateErr, ctx.Err())
	}

	if generateErr != nil {
		generateErr = fmt.Errorf("provider inference failed: %w", generateErr)
	}

	if response != nil {
		// Run the output guard before capturing the response if implemented.
		if guard, ok := p.runner.(task.OutputGuard); ok {
			if err := cancelAndTrace(ctx, "guard.output", func(context.Context) error {
				return guard.ProtectOutput(response)
			}); err != nil {
				return nil, errors.Join(generateErr, fmt.Errorf("protect inference output: %w", err))
			}
		}

		model := response.Model
		if model == "" {
			model = p.task.Model.Slug
		}
		p.output.Model = task.Model{Slug: model, Parameters: p.task.Model.Parameters}
		p.output.Attachments = append(p.output.Attachments, response.Attachments...)
		p.messages = append(p.messages, response.Output...)
		return response, generateErr
	}

	// There is a nil response.
	if generateErr == nil {
		generateErr = errors.ErrNoModelOutput
	}
	return nil, generateErr
}

// captureResponse stores the final answer while preserving any inference error.
func (p *process) captureResponse(response *provider.Response, inferenceErr error) error {
	if response == nil {
		if inferenceErr != nil {
			return inferenceErr
		}
		return errors.ErrNoModelOutput
	}

	// Only assistant text is task output; capability calls belong to CapabilitiesLoop.
	var finalContent, unphasedContent []string
	for _, message := range response.Output {
		if message == nil || message.Role != prompts.RoleAssistant || message.Type == prompts.TypeReasoning {
			continue
		}
		if message.Type == prompts.TypeToolCall || len(message.ToolCalls) > 0 {
			return errors.Join(inferenceErr, errors.ErrCapabilityProviderRequired)
		}
		if message.Content == "" {
			continue
		}

		switch message.Phase {
		case "final_answer":
			finalContent = append(finalContent, message.Content)
		case "":
			unphasedContent = append(unphasedContent, message.Content)
		}
	}

	// Prefer explicitly final text; use unphased assistant text as a fallback.
	content := finalContent
	if len(content) == 0 {
		content = unphasedContent
	}
	if len(content) == 0 {
		if len(response.Attachments) > 0 {
			return inferenceErr
		}
		return errors.Join(inferenceErr, errors.ErrNoModelOutput)
	}
	text := strings.Join(content, "\n")

	// Return the text as plain text if there is no schema requirement.
	if p.task.Output.Schema == nil || p.task.Output.Schema.MimeType.IsUnknown() {
		p.output.MimeType = mime.TextPlain
		p.output.Output = text
		return inferenceErr
	}

	// Parse the text against the schema, retaining raw text on failure.
	parsed, err := p.parser.Parse(text, p.task.Output.Schema)
	if err != nil {
		p.output.Output = text
		return errors.Join(inferenceErr, errors.ErrInvalidModelOutput, err)
	}
	p.output.Output = parsed
	p.output.MimeType = p.task.Output.Schema.MimeType
	return inferenceErr
}
