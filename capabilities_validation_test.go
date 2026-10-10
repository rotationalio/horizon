package horizon_test

//=============================================================================
// Tests capability setup, call correlation, arguments, and executor outcomes.
// Checks which failures stop execution and which let the model try again safely.
//=============================================================================

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
)

// Tries setup failures such as invalid selections, lookup errors, and a missing runner.
// Each case should stop before inference or tool execution, but still call Finalize with
// the setup error so the host can finish its lifecycle work.
func TestCapabilitiesSetupFailures(t *testing.T) {
	lookupErr := errors.New("private lookup failure")
	tests := []struct {
		name          string
		selected      []capabilities.Name
		definitions   []capabilities.Definition
		turns         *int64
		lookupErr     error
		cancelLookup  bool
		withoutRunner bool
		want          error
	}{
		{
			name:          "runner absent",
			withoutRunner: true,
			want:          errors.ErrCapabilityProviderRequired,
		},
		{
			name:  "zero turns",
			turns: capabilityTurns(0),
		},
		{
			name:  "negative turns",
			turns: capabilityTurns(-1),
		},
		{
			name:     "duplicate selection",
			selected: []capabilities.Name{"lookup", "lookup"},
		},
		{
			name:     "invalid selection",
			selected: []capabilities.Name{"invalid.name"},
			want:     capabilities.ErrInvalidName,
		},
		{
			name:      "lookup error",
			lookupErr: lookupErr,
			want:      lookupErr,
		},
		{
			name:         "cancelled lookup",
			cancelLookup: true,
			want:         context.Canceled,
		},
		{
			name:        "missing definition",
			definitions: []capabilities.Definition{},
			want:        capabilities.ErrNotFound,
		},
		{
			name:        "extra definition",
			definitions: []capabilities.Definition{{Name: "lookup"}, {Name: "extra"}},
		},
		{
			name:        "duplicate definition",
			definitions: []capabilities.Definition{{Name: "lookup"}, {Name: "lookup"}},
		},
		{
			name:        "invalid definition",
			definitions: []capabilities.Definition{{Name: "bad.name"}},
			want:        capabilities.ErrInvalidName,
		},
		{
			name:        "malformed schema",
			definitions: []capabilities.Definition{{Name: "lookup", InputSchema: &schema.Schema{Data: "{"}}},
		},
		{
			name:        "non-object schema",
			definitions: []capabilities.Definition{{Name: "lookup", InputSchema: &schema.Schema{Data: `{"type":"string"}`}}},
			want:        errors.ErrInvalidJSONSchema,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: one invalid setup condition; the provider must remain unused.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			selected := tt.selected
			if selected == nil {
				selected = []capabilities.Name{"lookup"}
			}
			runner := newCapabilityTestRunner(selected...)
			prepare := runner.Runner.OnPrepare
			runner.Runner.OnPrepare = func(ctx context.Context, definition *task.Task) error {
				if err := prepare(ctx, definition); err != nil {
					return err
				}
				if tt.turns != nil {
					definition.Model.Parameters = params.New(map[string]any{params.MaxToolTurns: *tt.turns})
				}
				return nil
			}
			runner.CapabilityRunner.OnLookupCapabilities = func(context.Context, []capabilities.Name) ([]capabilities.Definition, error) {
				if tt.cancelLookup {
					cancel()
				}
				if tt.definitions != nil {
					return tt.definitions, tt.lookupErr
				}
				return []capabilities.Definition{{Name: "lookup"}}, tt.lookupErr
			}
			var host task.Runner = runner
			if tt.withoutRunner {
				host = runner.Runner
			}
			inference := mockprovider.New(ulid.Make())

			// Execute setup through the lifecycle, including cleanup on failure.
			output, err := runProcessTest(t, ctx, inference, host, &schema.Output{})

			// Assert no inference or side effects occurred, and the cause is preserved.
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
			require.Zero(t, output.Usage.Invocations)
			inference.AssertNotCalled(t, mockprovider.Generate)
			runner.CapabilityRunner.AssertNotCalled(t, mock.ExecuteCapability)
			runner.Runner.AssertCalled(t, mock.Finalize, 1)
		})
	}
}

// Supplies batches with missing, duplicate, or otherwise invalid call IDs, including IDs
// already present in conversation history. Horizon must validate the whole batch first so
// no capability in a malformed batch is executed as a partial side effect.
func TestCapabilitiesBatchValidation(t *testing.T) {
	tests := []struct {
		name     string
		messages prompts.Prompts
		initial  bool
		want     error
	}{
		{
			name:     "missing second ID",
			messages: capabilityTestResponse(prompts.ToolCall{CallID: "first", Name: "lookup", Arguments: `{}`}, prompts.ToolCall{Name: "lookup", Arguments: `{}`}).Output,
			want:     errors.ErrToolCallIDRequired,
		},
		{
			name:     "duplicate IDs",
			messages: capabilityTestResponse(prompts.ToolCall{CallID: "same", Name: "lookup", Arguments: `{}`}, prompts.ToolCall{CallID: "same", Name: "lookup", Arguments: `{}`}).Output,
			want:     capabilities.ErrInvalidToolPrompt,
		},
		{
			name:     "tool marker without calls",
			messages: prompts.Prompts{nil, {Role: prompts.RoleAssistant, Type: prompts.TypeToolCall}},
			want:     capabilities.ErrUnsupportedResponse,
		},
		{
			name:     "non-assistant call",
			messages: prompts.Prompts{{Role: prompts.RoleUser, ToolCalls: []prompts.ToolCall{{CallID: "call", Name: "lookup", Arguments: `{}`}}}},
			want:     capabilities.ErrUnsupportedResponse,
		},
		{
			name:     "broken initial transcript",
			messages: prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeToolCall}},
			initial:  true,
			want:     capabilities.ErrUnsupportedResponse,
		},
		{
			name:     "initial ID absent",
			messages: capabilityTestResponse(prompts.ToolCall{Name: "lookup", Arguments: `{}`}).Output,
			initial:  true,
			want:     errors.ErrToolCallIDRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: malformed input is supplied either by a renderer or the provider.
			runner := newCapabilityTestRunner("lookup")
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				response := processTestResponse()
				response.Output = tt.messages
				return response, nil
			}
			var host task.Runner = runner
			if tt.initial {
				host = struct {
					*capabilityTestRunner
					*mock.Renderer
				}{runner, &mock.Renderer{OnRender: func(prompts.Templates, prompts.Context) (prompts.Prompts, error) { return tt.messages, nil }}}
			}

			// Execute; validation must complete before any call in the batch dispatches.
			output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{})

			// Assert even the otherwise valid first call was not executed.
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, output.Output)
			runner.CapabilityRunner.AssertNotCalled(t, mock.ExecuteCapability)
			if tt.initial {
				inference.AssertNotCalled(t, mockprovider.Generate)
			}
		})
	}
}

// Tries valid and invalid capability names and JSON arguments. Invalid requests get a
// safe result correlated to the model's call so it can correct itself, without dispatching
// the request to the host; valid objects are passed to the executor.
func TestCapabilitiesArgumentRecovery(t *testing.T) {
	const (
		unavailableResult = "The requested capability is unavailable. Choose an advertised capability."
		invalidArgsResult = "Arguments must be one valid JSON object. Correct them and try again."
		executorResult    = "value found"
	)
	tests := []struct {
		name, tool, arguments string
		wantDispatch          bool
		wantArguments         map[string]any
		wantResult            string
	}{
		// These names cannot identify one of the capabilities advertised to the model.
		{
			name:       "name is not advertised",
			tool:       "private_route",
			arguments:  `{}`,
			wantResult: unavailableResult,
		},
		{
			name:       "name has invalid characters",
			tool:       "bad.name",
			arguments:  `{}`,
			wantResult: unavailableResult,
		},
		{
			name:       "name is empty",
			arguments:  `{}`,
			wantResult: unavailableResult,
		},

		// These inputs are not one valid JSON object, so the executor must not run.
		{
			name:       "arguments are missing",
			tool:       "lookup",
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are whitespace",
			tool:       "lookup",
			arguments:  " \t\n",
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are a string, not an object",
			tool:       "lookup",
			arguments:  `"value"`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are a number, not an object",
			tool:       "lookup",
			arguments:  `42`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments contain malformed JSON",
			tool:       "lookup",
			arguments:  `{"value":`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are an array, not an object",
			tool:       "lookup",
			arguments:  `[]`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are null, not an object",
			tool:       "lookup",
			arguments:  `null`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments are a boolean, not an object",
			tool:       "lookup",
			arguments:  `true`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments contain a second JSON value",
			tool:       "lookup",
			arguments:  `{} {}`,
			wantResult: invalidArgsResult,
		},
		{
			name:       "arguments contain trailing junk",
			tool:       "lookup",
			arguments:  `{} junk`,
			wantResult: invalidArgsResult,
		},

		// These objects are accepted and their decoded values reach the executor.
		{
			name:          "valid object arguments",
			tool:          "lookup",
			arguments:     " {\"value\":\"expected\"} \n",
			wantDispatch:  true,
			wantArguments: map[string]any{"value": "expected"},
			wantResult:    executorResult,
		},
		{
			name:         "nested object with mixed JSON values",
			tool:         "lookup",
			arguments:    `{"nested":{"ratio":-0.125,"list":[1e30,null,true,"é"]}}`,
			wantDispatch: true,
			wantArguments: map[string]any{
				"nested": map[string]any{
					"ratio": float64(-0.125),
					"list":  []any{float64(1e30), nil, true, "é"},
				},
			},
			wantResult: executorResult,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each case asks the model for one call, then supplies a final answer after
			// Horizon either sends a correction or returns the executor's result.
			runner := newCapabilityTestRunner("lookup")
			runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
				require.True(t, tt.wantDispatch, "only valid object arguments should reach the executor")
				require.Equal(t, tt.wantArguments, request.Arguments)
				return capabilities.NewToolTextContentResponse(executorResult), nil
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if turn == 1 {
					return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: tt.tool, Arguments: tt.arguments}), nil
				}
				require.Len(t, request.Input, 2)
				require.Equal(t, tt.arguments, request.Input[0].ToolCalls[0].Arguments)
				require.Equal(t, "call", request.Input[1].ToolResults[0].CallID)
				require.Equal(t, tt.wantResult, request.Input[1].ToolResults[0].Content)
				return processTestResponse(), nil
			}

			// Execute recovery; Horizon supplies a result, but does not retry the executor.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

			// Assert final-only output and whether the executor ran for this input.
			require.NoError(t, err)
			require.Equal(t, "answer", output.Output)
			require.Equal(t, uint64(2), output.Usage.Invocations)
			if tt.wantDispatch {
				require.Equal(t, uint64(1), output.Usage.ToolCalls)
				runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
			} else {
				require.Zero(t, output.Usage.ToolCalls)
				runner.CapabilityRunner.AssertNotCalled(t, mock.ExecuteCapability)
			}
		})
	}
}

// Exercises successful results, retryable and terminal errors, and response kinds that
// Horizon does not support. Recoverable outcomes become safe correlated messages for the
// model; terminal errors stop execution and keep diagnostic causes private to the host.
func TestCapabilitiesExecutorOutcomes(t *testing.T) {
	cause := errors.New("secret credential and private route")
	tests := []struct {
		name         string
		response     capabilities.Response
		err          *capabilities.Error
		wantTerminal bool
	}{
		{
			name:     "supported",
			response: capabilities.NewToolTextContentResponse("safe result"),
		},
		{
			name: "retryable",
			err:  &capabilities.Error{Retryable: true, Message: "try a different query", Cause: cause},
		},
		{
			name:         "terminal",
			err:          &capabilities.Error{Message: "execution failed", Cause: cause},
			wantTerminal: true,
		},
		{
			name:     "retryable with conflicting result",
			response: capabilities.NewToolTextContentResponse("unsafe diagnostic text"),
			err:      &capabilities.Error{Retryable: true, Message: "correct the request", Cause: cause},
		},
		{
			name:         "terminal with conflicting result",
			response:     capabilities.NewToolTextContentResponse("unsafe diagnostic text"),
			err:          &capabilities.Error{Message: "stop execution", Cause: cause},
			wantTerminal: true,
		},
		{
			name: "retryable without a cause",
			err:  &capabilities.Error{Retryable: true, Message: "try another query"},
		},
		{
			name:         "terminal without a cause",
			err:          &capabilities.Error{Message: "stop execution"},
			wantTerminal: true,
		},
		// Empty text is valid when the executor explicitly selects the supported kind.
		{
			name:     "explicit empty text result allows continuation",
			response: capabilities.NewToolTextContentResponse(""),
		},
		// Missing or unsupported kinds produce a safe fallback and allow model recovery.
		{
			name:     "zero value returns an unsupported result",
			response: capabilities.Response{},
		},
		{
			name: "unrecognized response kind asks the model how to proceed",
			response: capabilities.Response{
				Kind:    capabilities.ResponseKind(255),
				Content: "unsafe",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: the executor contributes a response or a classified failure.
			runner := newCapabilityTestRunner("lookup")
			runner.CapabilityRunner.OnExecuteCapability = func(context.Context, capabilities.Request) (capabilities.Response, *capabilities.Error) {
				return tt.response, tt.err
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if turn == 1 {
					return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: "lookup", Arguments: `{}`}), nil
				}
				require.False(t, tt.wantTerminal, "terminal executor errors must stop before another inference")
				result := request.Input[1].ToolResults[0]
				require.Equal(t, "call", result.CallID)
				require.NotContains(t, result.Content, cause.Error())
				if tt.err != nil {
					require.Equal(t, tt.err.Message, result.Content)
				} else if tt.response.Kind == capabilities.ResponseKindToolTextContent {
					require.Equal(t, tt.response.Content, result.Content)
				} else {
					require.Equal(t, "The capability returned an unsupported result.", result.Content)
				}
				require.NotContains(t, result.Content, "unsafe")
				return processTestResponse(), nil
			}

			// Execute; successful results and recoverable failures allow another model
			// turn, but terminal executor errors stop the loop.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

			// Assert diagnostic causes are preserved for terminal errors, never sent to the model.
			if tt.wantTerminal {
				require.ErrorIs(t, err, tt.err)
				if tt.err.Cause != nil {
					require.ErrorIs(t, err, cause)
				}
				require.Nil(t, output.Output)
				require.Len(t, output.Messages, 1)
				require.Equal(t, 1, turn)
			} else {
				require.NoError(t, err)
				require.Equal(t, "answer", output.Output)
				require.Len(t, output.Messages, 3)
				require.Equal(t, 2, turn, "recoverable outcomes should allow another model turn")
			}
			require.Equal(t, uint64(1), output.Usage.ToolCalls)
			runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
		})
	}
}
