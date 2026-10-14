package horizon_test

//=============================================================================
// Tests how the capabilities loop builds and replays conversations, applies
// guards, follows model-directed retries, and keeps concurrent runs isolated.
//=============================================================================

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/prompts/templates"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

// Runs a complete capabilities exchange: render the initial question, handle two batches
// of tool calls, and then finish with an assistant answer. The test checks that calls and
// results stay in order, guard edits stay isolated, and intermediate text is not returned.
func TestCapabilitiesConversation(t *testing.T) {
	// Setup: compose the existing mocks and script two capability batches followed
	// by a final answer. No real model or executor decides the test's behavior.
	runner := newCapabilityTestRunner("lookup", "other")
	renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
		return prompts.Prompts{{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question"}}, nil
	}}
	composed := struct {
		*capabilityTestRunner
		*mock.Renderer
	}{runner, renderer}

	// Return definitions out of order and alter the lookup's selection slice.
	// Neither action should change the task's selection or its advertised order.
	runner.CapabilityRunner.OnLookupCapabilities = func(_ context.Context, names []capabilities.Name) ([]capabilities.Definition, error) {
		// Lookup order may differ from selection order, and the lookup input is owned.
		names[0] = "mutated_lookup_input"
		return []capabilities.Definition{
			{Name: "other"},
			{Name: "lookup", InputSchema: &schema.Schema{Data: `{"type":"object"}`}},
		}, nil
	}

	// Input validation receives a snapshot, so these edits must never reach the model.
	inputTurns := 0
	runner.InputGuard.OnProtectInput = func(request *provider.Request) error {
		inputTurns++
		// Try to replace conversation state and corrupt advertised definitions. The
		// provider must receive the untouched request, and later turns must use the
		// original transcript rather than these validation-only edits.
		request.Input[0].Content = fmt.Sprintf("guarded question %d", inputTurns)
		request.Tools[0].Name = "private_route"
		request.Tools[0].InputSchema.Data = "corrupted"
		return nil
	}

	// Output guards can change accepted responses. Change the third call's arguments
	// before execution, and replace the final answer before it is returned.
	runner.OutputGuard.OnProtectOutput = func(response *provider.Response) error {
		if len(response.Output[0].ToolCalls) == 0 {
			response.Output[0].Content = "guarded final answer"
		} else if response.Output[0].ToolCalls[0].CallID == "call-3" {
			response.Output[0].ToolCalls[0].Arguments = `{"id":42}`
		}
		return nil
	}

	// Record dispatch order and check that the executor sees the output guard's edit.
	var executed []string
	runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
		executed = append(executed, request.CallID)
		if request.CallID == "call-3" {
			require.Equal(t, float64(42), request.Arguments["id"])
		}
		return capabilities.NewToolTextContentResponse("result " + request.CallID), nil
	}

	// Script three inference turns: two calls, one more call, then the final answer.
	// Each continuation checks the history assembled from the preceding turn.
	turn := 0
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		turn++
		require.Equal(t, "question", request.Input[0].Content)
		require.Equal(t, capabilities.Name("lookup"), request.Tools[0].Name)
		require.Equal(t, capabilities.Name("other"), request.Tools[1].Name)
		require.Equal(t, `{"type":"object"}`, request.Tools[0].InputSchema.Data)
		switch turn {
		case 1:
			require.Len(t, request.Input, 1)
			response := capabilityTestResponse(
				prompts.ToolCall{CallID: "call-1", Name: "lookup", Arguments: `{}`},
				prompts.ToolCall{CallID: "call-2", Name: "other", Arguments: `{}`},
			)
			response.Output = append(prompts.Prompts{{ID: "reason-1", Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Content: "reasoning"}}, response.Output...)
			return response, nil
		case 2:
			require.Len(t, request.Input, 5)
			require.Equal(t, "reason-1", request.Input[1].ID)
			require.Equal(t, []string{"call-1", "call-2"}, executed)
			require.Equal(t, "call-1", request.Input[3].ToolResults[0].CallID)
			require.Equal(t, "call-2", request.Input[4].ToolResults[0].CallID)
			return capabilityTestResponse(prompts.ToolCall{CallID: "call-3", Name: "lookup", Arguments: `{"id":0}`}), nil
		default:
			require.Equal(t, 3, turn, "the final answer must stop inference")
			require.Len(t, request.Input, 7)
			require.Equal(t, `{"id":42}`, request.Input[5].ToolCalls[0].Arguments)
			require.Equal(t, "call-3", request.Input[6].ToolResults[0].CallID)
			return processTestResponse(), nil
		}
	}

	// Finalize receives the final answer and the complete, unfiltered transcript.
	runner.Runner.OnFinalize = func(ctx context.Context, output *task.Output, executionErr error) error {
		require.NoError(t, ctx.Err())
		require.NoError(t, executionErr)
		require.Equal(t, "guarded final answer", output.Output)
		require.Len(t, output.Messages(), 8, "requires all messages to pass through filtering")
		return nil
	}

	// Execute through the same Horizon.Run entry point used by applications.
	output, err := runProcessTest(t, context.Background(), inference, composed, &schema.Output{})

	// Assert final-only output, cumulative usage, sequential execution, and one-time setup.
	require.NoError(t, err)
	require.Equal(t, "guarded final answer", output.Output)
	require.Equal(t, uint64(3), output.Usage.Invocations)
	require.Equal(t, uint64(3), output.Usage.ToolCalls)
	require.Equal(t, int64(54), output.Usage.TotalTokens)
	require.Equal(t, 0.75, output.Usage.APICost)
	require.Equal(t, []string{"call-1", "call-2", "call-3"}, executed)
	runner.CapabilityRunner.AssertCalled(t, mock.LookupCapabilities, 1)
	runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 3)
	runner.InputGuard.AssertCalled(t, mock.ProtectInput, 3)
	runner.OutputGuard.AssertCalled(t, mock.ProtectOutput, 3)
	renderer.AssertCalled(t, mock.Render, 1)
	runner.Runner.AssertCalled(t, mock.Finalize, 1)
}

// Gives the input guard a request containing mutable prompts, schemas, attachments, and
// tool definitions, then has the guard change them. The provider should still receive the
// original request on each turn, because guard edits are for validation only.
func TestCapabilitiesInputGuardMutationsIgnored(t *testing.T) {
	// Setup: include mutable data in every request component exposed to the guard.
	metadata := map[string]any{"nested": map[string]any{"labels": []any{"original"}}}
	user := &prompts.Prompt{
		Role:      prompts.RoleUser,
		Type:      prompts.TypeMessage,
		Content:   "question",
		Citations: []prompts.Citation{{Title: "source"}},
		Meta:      metadata,
	}
	assistantCall := &prompts.Prompt{
		Role:      prompts.RoleAssistant,
		Type:      prompts.TypeToolCall,
		ToolCalls: []prompts.ToolCall{{CallID: "prior-call", Name: "lookup", Arguments: `{}`}},
	}
	toolResult := &prompts.Prompt{
		Role:        prompts.RoleTool,
		Type:        prompts.TypeMessage,
		ToolResults: []prompts.ToolResponse{{CallID: "prior-call", Content: "prior result"}},
	}
	file := &attachments.Attachment{Filename: "input.txt", Data: []byte("abc")}
	input := prompts.Prompts{user, assistantCall, toolResult}

	// Supply model parameters and a tool schema so the guard can attempt to
	// mutate those fields as well as the existing conversation and attachment.
	runner := newCapabilityTestRunner("lookup")
	runner.Runner.OnPrepare = func(_ context.Context, definition *task.Task) error {
		definition.Capabilities = []capabilities.Name{"lookup"}
		definition.Model.Parameters = params.New(map[string]any{params.Temperature: 0.5})
		return nil
	}
	runner.CapabilityRunner.OnLookupCapabilities = func(context.Context, []capabilities.Name) ([]capabilities.Definition, error) {
		return []capabilities.Definition{{
			Name:        "lookup",
			InputSchema: &schema.Schema{Data: `{"type":"object"}`},
		}}, nil
	}

	// Deliberately corrupt the snapshot, then replace its top-level fields too.
	// Both kinds of edits must remain isolated from the real provider request.
	runner.InputGuard.OnProtectInput = func(request *provider.Request) error {
		// Mutate scalar fields, nested prompt data, slices, and pointed-to values.
		request.Model = "guarded-model"
		request.Params.Set(params.Temperature, 1.0)
		request.Input[0].Content = "guarded question"
		request.Input[0].Citations[0].Title = "guarded source"
		labels := request.Input[0].Meta["nested"].(map[string]any)["labels"].([]any)
		labels[0] = "guarded label"
		request.Input[1].ToolCalls[0].Arguments = `{"changed":true}`
		request.Input[2].ToolResults[0].Content = "guarded result"
		request.Attachments[0].Filename = "guarded.txt"
		request.Attachments[0].Data[0] = 'x'
		request.OutputSchema.Data = "guarded output schema"
		request.Tools[0].Name = "private_route"
		request.Tools[0].InputSchema.Data = "guarded tool schema"

		// Replacing top-level fields must also stay inside the guard's copy.
		request.Params = nil
		request.Input = nil
		request.Attachments = nil
		request.OutputSchema = nil
		request.Tools = nil
		return nil
	}

	// Render the existing conversation and check the provider's request after
	// input validation. Its nested data should still match the original setup.
	renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
		return input, nil
	}}
	host := struct {
		*capabilityTestRunner
		*mock.Renderer
	}{runner, renderer}
	outputSchema := &schema.Output{Schema: &schema.Schema{MimeType: mime.TextPlain, Data: "original output schema"}}
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		// Assert: provider input matches the canonical request, including nested fields.
		require.Equal(t, "requested-model", request.Model)
		value, ok := request.Params.Get(params.Temperature)
		require.True(t, ok)
		require.Equal(t, 0.5, value)
		require.Equal(t, input, request.Input)
		require.Equal(t, "original", request.Input[0].Meta["nested"].(map[string]any)["labels"].([]any)[0])
		require.Equal(t, "{}", request.Input[1].ToolCalls[0].Arguments)
		require.Equal(t, "prior result", request.Input[2].ToolResults[0].Content)
		require.Equal(t, "input.txt", request.Attachments[0].Filename)
		require.Equal(t, "abc", string(request.Attachments[0].Data))
		require.Equal(t, "original output schema", request.OutputSchema.Data)
		require.Equal(t, capabilities.Name("lookup"), request.Tools[0].Name)
		require.Equal(t, `{"type":"object"}`, request.Tools[0].InputSchema.Data)
		return processTestResponse(), nil
	}

	// Execute one inference through the complete capabilities setup and guard path.
	output, err := runProcessTest(t, context.Background(), inference, host, outputSchema, &task.Input{Attachments: attachments.Attachments{file}})

	// Assert guard mutations never escaped their snapshot or changed host-owned input.
	require.NoError(t, err)
	require.Equal(t, "question", user.Content)
	require.Equal(t, "source", user.Citations[0].Title)
	require.Equal(t, "original", metadata["nested"].(map[string]any)["labels"].([]any)[0])
	require.Equal(t, "abc", string(file.Data))
	require.Equal(t, "answer", output.Output)
	runner.InputGuard.AssertCalled(t, mock.ProtectInput, 1)
	inference.AssertCalled(t, mockprovider.Generate, 1)
}

// Makes the first executor call fail in a retryable way, then has the model issue a new
// call with corrected input. This verifies Horizon does not retry the executor by itself;
// the second execution happens only because the model requested another call.
func TestCapabilitiesModelDirectedRetry(t *testing.T) {
	// Setup: reject the first executor call and accept a corrected second call.
	runner := newCapabilityTestRunner("lookup")
	cause := errors.New("private failure")
	var calls []string
	runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
		calls = append(calls, request.CallID)
		if request.CallID == "first" {
			return capabilities.Response{}, &capabilities.Error{Retryable: true, Message: "correct the query", Cause: cause}
		}
		return capabilities.NewToolTextContentResponse("found"), nil
	}
	turn := 0
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		turn++
		switch turn {
		case 1:
			return capabilityTestResponse(prompts.ToolCall{CallID: "first", Name: "lookup", Arguments: `{}`}), nil
		case 2:
			require.Equal(t, []string{"first"}, calls, "Horizon must not automatically retry the executor")
			require.Equal(t, "correct the query", request.Input[1].ToolResults[0].Content)
			return capabilityTestResponse(prompts.ToolCall{CallID: "corrected", Name: "lookup", Arguments: `{}`}), nil
		default:
			require.Equal(t, "found", request.Input[3].ToolResults[0].Content)
			return processTestResponse(), nil
		}
	}

	// Execute three inference turns, with only the two model-requested side effects.
	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	// Assert the diagnostic cause stayed private and each call ran exactly once.
	require.NoError(t, err)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, []string{"first", "corrected"}, calls)
	require.Len(t, output.Messages(), 5)
	require.NotContains(t, output.Messages()[1].ToolResults[0].Content, cause.Error())
	require.Equal(t, uint64(2), output.Usage.ToolCalls)
}

// Checks both a task that selects capabilities but receives a normal answer immediately,
// and a task with no selected capabilities. The first advertises definitions in one turn;
// the second skips capability lookup entirely, and both finish successfully.
func TestCapabilitiesSelectionWithoutCalls(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected=%t", selected), func(t *testing.T) {
			// Setup: a capability-capable runner and a provider returning only an answer.
			runner := newCapabilityTestRunner()
			if selected {
				runner = newCapabilityTestRunner("lookup")
			}
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				return processTestResponse(), nil
			}

			// Execute through Run so the one-shot/capabilities branch is exercised.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

			// Assert setup runs only for a nonempty selection, with no executor dispatch.
			require.NoError(t, err)
			require.Equal(t, "answer", output.Output)
			require.Equal(t, uint64(1), output.Usage.Invocations)
			runner.CapabilityRunner.AssertNotCalled(t, mock.ExecuteCapability)
			if selected {
				runner.CapabilityRunner.AssertCalled(t, mock.LookupCapabilities, 1)
			} else {
				runner.CapabilityRunner.AssertNotCalled(t, mock.LookupCapabilities)
			}
		})
	}
}

// Runs several capability loops concurrently using shared test infrastructure. Each run
// should keep its own selected capability, call IDs, and conversation history rather than
// observing another execution's state.
func TestCapabilitiesConcurrentIsolation(t *testing.T) {
	const runs = 8
	type executionKey struct{}

	// Setup: one shared provider; each runner supplies its own selected capability.
	configuration := testProviderConfig()
	inference := mockprovider.New(configuration.ID)
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		require.Equal(t, "lookup_"+request.Model, string(request.Tools[0].Name))
		require.Equal(t, request.Model, request.Input[0].Content)
		if len(request.Input) == 1 {
			return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: string(request.Tools[0].Name), Arguments: `{}`}), nil
		}
		require.Len(t, request.Input, 3)
		require.Equal(t, request.Model, request.Input[2].ToolResults[0].Content)
		response := processTestResponse()
		response.Output[0].Content = request.Model
		return response, nil
	}
	h := newTestHorizon(t, testFactory(func(ctx context.Context) (task.Runner, error) {
		id := ctx.Value(executionKey{}).(string)
		runner := newCapabilityTestRunner(capabilities.Name("lookup_" + id))
		prepare := runner.Runner.OnPrepare
		runner.Runner.OnPrepare = func(ctx context.Context, definition *task.Task) error {
			if err := prepare(ctx, definition); err != nil {
				return err
			}
			definition.Model.Slug = id
			return nil
		}
		renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
			return prompts.Prompts{{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: id}}, nil
		}}
		runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
			require.Equal(t, capabilities.Name("lookup_"+id), request.Name)
			return capabilities.NewToolTextContentResponse(id), nil
		}
		host := struct {
			*capabilityTestRunner
			*mock.Renderer
		}{runner, renderer}
		return host, nil
	}))
	_, err := h.GetOrCreateProvider(configuration, inference)
	require.NoError(t, err)
	definition := &task.Task{Provider: &configuration, Output: &schema.Output{}}
	type result struct {
		id     string
		output *task.Output
		err    error
	}
	results := make(chan result, runs)

	// Execute concurrent runs against the same task definition and inference client.
	for i := 0; i < runs; i++ {
		id := fmt.Sprintf("run%d", i)
		go func() {
			ctx := context.WithValue(context.Background(), executionKey{}, id)
			output, err := h.Run(ctx, &task.Input{}, definition)
			results <- result{id: id, output: output, err: err}
		}()
	}

	// Assert each returned transcript and selection belongs only to its execution.
	for i := 0; i < runs; i++ {
		got := <-results
		require.NoError(t, got.err)
		require.Equal(t, got.id, got.output.Output)
		require.Equal(t, []capabilities.Name{capabilities.Name("lookup_" + got.id)}, got.output.Capabilities)
		require.Equal(t, got.id, got.output.Messages()[0].Content)
		require.Equal(t, uint64(2), got.output.Usage.Invocations)
		require.Equal(t, uint64(1), got.output.Usage.ToolCalls)
	}
	require.Empty(t, definition.Capabilities)
	inference.AssertCalled(t, mockprovider.Generate, runs*2)
}

// Starts with a transcript that already contains a tool call and its result. Horizon
// should replay that history without executing the old call again, keep its ID reserved,
// and preserve the order and identity of messages as new calls are added.
func TestCapabilitiesExistingHistory(t *testing.T) {
	for _, reuseID := range []bool{false, true} {
		t.Run(fmt.Sprintf("reuse prior ID=%t", reuseID), func(t *testing.T) {
			// Setup: a renderer supplies a completed historical call and its result.
			history := prompts.Prompts{
				{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question"},
				{ID: "prior-item", Role: prompts.RoleAssistant, Type: prompts.TypeToolCall, ToolCalls: []prompts.ToolCall{{CallID: "prior", Name: "lookup", Arguments: `{}`}}},
				{Role: prompts.RoleTool, Type: prompts.TypeMessage, ToolResults: []prompts.ToolResponse{{CallID: "prior", Content: "historical result"}}},
			}
			runner := newCapabilityTestRunner("lookup")
			var dispatched []string
			runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
				dispatched = append(dispatched, request.CallID)
				return capabilities.NewToolTextContentResponse("current result"), nil
			}
			host := struct {
				*capabilityTestRunner
				*mock.Renderer
			}{
				runner,
				&mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) { return history, nil }},
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if turn == 1 {
					require.Equal(t, history, request.Input)
					secondID := "new-2"
					if reuseID {
						secondID = "prior"
					}
					response := processTestResponse()
					response.Output = prompts.Prompts{
						{ID: "new-item-1", Role: prompts.RoleAssistant, Type: prompts.TypeToolCall, ToolCalls: []prompts.ToolCall{{CallID: "new-1", Name: "lookup", Arguments: `{}`}}},
						{ID: "new-item-2", Role: prompts.RoleAssistant, Type: prompts.TypeToolCall, ToolCalls: []prompts.ToolCall{{CallID: secondID, Name: "lookup", Arguments: `{}`}}},
					}
					return response, nil
				}
				require.False(t, reuseID, "a reused historical ID must terminate before dispatch")
				require.Len(t, request.Input, 7)
				require.Equal(t, "new-item-1", request.Input[3].ID)
				require.Equal(t, "new-item-2", request.Input[4].ID)
				require.Equal(t, "new-1", request.Input[5].ToolResults[0].CallID)
				require.Equal(t, "new-2", request.Input[6].ToolResults[0].CallID)
				return processTestResponse(), nil
			}

			// Execute with pre-existing conversation, not freshly rendered text alone.
			output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{})

			// Assert no historical side effect was repeated, and new calls kept their grouping.
			if reuseID {
				require.ErrorIs(t, err, capabilities.ErrInvalidToolPrompt)
				require.Empty(t, dispatched, "the valid first call must not run before whole-batch validation")
				require.Len(t, output.Messages(), 5)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"new-1", "new-2"}, dispatched)
				require.Len(t, output.Messages(), 8)
			}
			require.Len(t, history, 3)
			require.Equal(t, "historical result", output.Messages()[2].ToolResults[0].Content)
		})
	}
}

// Runs the loop with no guards, each guard separately, and both together. This checks
// that Horizon discovers each optional guard independently, ignores input-guard edits to
// the provider request, and keeps output-guard changes across the tool exchange.
func TestCapabilitiesOptionalGuards(t *testing.T) {
	for _, guards := range []string{"none", "input only", "output only", "both"} {
		t.Run(guards, func(t *testing.T) {
			// Setup: expose only the chosen interfaces using the existing mock components.
			runner := newCapabilityTestRunner("lookup")
			runner.InputGuard.OnProtectInput = func(request *provider.Request) error { request.Model = "guarded-model"; return nil }
			runner.OutputGuard.OnProtectOutput = func(response *provider.Response) error {
				if len(response.Output[0].ToolCalls) == 0 {
					response.Output[0].Content = "guarded answer"
				}
				return nil
			}
			var host task.Runner = struct {
				*mock.Runner
				*mock.CapabilityRunner
			}{runner.Runner, runner.CapabilityRunner}
			switch guards {
			case "input only":
				host = struct {
					*mock.Runner
					*mock.CapabilityRunner
					*mock.InputGuard
				}{runner.Runner, runner.CapabilityRunner, runner.InputGuard}
			case "output only":
				host = struct {
					*mock.Runner
					*mock.CapabilityRunner
					*mock.OutputGuard
				}{runner.Runner, runner.CapabilityRunner, runner.OutputGuard}
			case "both":
				host = runner
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				require.Equal(t, "requested-model", request.Model)
				if turn == 1 {
					return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: "lookup", Arguments: `{}`}), nil
				}
				require.Len(t, request.Input, 2)
				return processTestResponse(), nil
			}

			// Execute a call/result exchange and a final inference for every interface combination.
			output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{})

			// Assert each guard runs once per turn only when its interface is exposed.
			require.NoError(t, err)
			if guards == "input only" || guards == "both" {
				runner.InputGuard.AssertCalled(t, mock.ProtectInput, 2)
			} else {
				runner.InputGuard.AssertNotCalled(t, mock.ProtectInput)
			}
			if guards == "output only" || guards == "both" {
				runner.OutputGuard.AssertCalled(t, mock.ProtectOutput, 2)
				require.Equal(t, "guarded answer", output.Output)
			} else {
				runner.OutputGuard.AssertNotCalled(t, mock.ProtectOutput)
				require.Equal(t, "answer", output.Output)
			}
			runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
		})
	}
}
