package horizon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

// Verifies that a successful inference is guarded, accounted, and finalized in order.
func TestProcessRunSuccessfulInference(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	runner := &processTestRunner{}
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		runner.events = append(runner.events, "generate")
		require.Equal(t, "requested-model", request.Model)
		return processTestResponse(), nil
	}
	runner.protectOutput = func(response *provider.Response) error {
		response.Output[0].Content = "guarded answer"
		return nil
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "guarded answer", output.Output)
	require.Equal(t, mime.TextPlain, output.MimeType)
	require.Equal(t, "served-model", output.Model.Slug)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(11), output.Usage.InputTokens)
	require.Equal(t, int64(7), output.Usage.OutputTokens)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, 0.25, output.Usage.APICost)
	require.Equal(t, task.OutcomeSucceeded, output.Outcome)
	require.False(t, output.Started.IsZero())
	require.False(t, output.Finished.IsZero())
	require.Equal(t, []string{"prepare", "input guard", "generate", "output guard", "finalize"}, runner.events)
	require.Equal(t, 1, inference.Calls(t, mockprovider.Generate))
}

// A finalization failure updates a successful execution's returned outcome.
func TestProcessRunFinalizeFailureMarksOutputFailed(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		finalizeErr: errors.New("finalize failed"),
		finalize: func(_ context.Context, output *task.Output, executionErr error) error {
			require.NoError(t, executionErr)
			require.Equal(t, task.OutcomeSucceeded, output.Outcome)
			return nil
		},
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, runner.finalizeErr)
	require.Equal(t, task.OutcomeFailed, output.Outcome)
}

// Ensures a provider failure retains returned partial output and usage for the host.
func TestProcessRunReturnsPartialOutputOnInferenceFailure(t *testing.T) {
	providerErr := errors.New("provider failed after producing a partial response")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), providerErr
	}
	runner := &processTestRunner{}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, providerErr)
	require.NotNil(t, output)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, task.OutcomeFailed, output.Outcome)
	require.Equal(t, []string{"prepare", "input guard", "output guard", "finalize"}, runner.events)
}

// Confirms that preparation and finalization failures are both returned without
// discarding the execution output.
func TestProcessRunPreservesPreparationAndFinalizationFailures(t *testing.T) {
	prepareErr := errors.New("prepare failed")
	finalizeErr := errors.New("finalize failed")
	runner := &processTestRunner{
		prepareErr:  prepareErr,
		finalizeErr: finalizeErr,
		finalize: func(_ context.Context, output *task.Output, executionErr error) error {
			require.ErrorIs(t, executionErr, prepareErr)
			require.Equal(t, task.OutcomeFailed, output.Outcome)
			return nil
		},
	}
	output, err := runProcessTest(t, context.Background(), mockprovider.New(ulid.Make()), runner, &schema.Output{})

	require.ErrorIs(t, err, prepareErr)
	require.ErrorIs(t, err, finalizeErr)
	require.NotNil(t, output)
	require.False(t, output.Started.IsZero())
	require.False(t, output.Finished.IsZero())
	require.Equal(t, task.OutcomeFailed, output.Outcome)
	require.Equal(t, []string{"prepare", "finalize"}, runner.events)
}

// Ensures a rejected request never reaches the provider and still reaches finalization.
func TestProcessRunStopsWhenInputGuardRejects(t *testing.T) {
	guardErr := errors.New("request rejected")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		require.FailNow(t, "provider must not be called when input is rejected")
		return nil, nil
	}
	runner := &processTestRunner{
		protectInput: func(*provider.Request) error { return guardErr },
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, guardErr)
	require.NotNil(t, output)
	require.Zero(t, output.Usage.Invocations)
	require.Equal(t, 0, inference.Calls(t, mockprovider.Generate))
	require.Equal(t, []string{"prepare", "input guard", "finalize"}, runner.events)
}

// Ensures output-guard failures block model content while retaining provider usage.
func TestProcessRunStopsWhenOutputGuardRejects(t *testing.T) {
	guardErr := errors.New("response rejected")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		protectOutput: func(*provider.Response) error { return guardErr },
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, guardErr)
	require.NotNil(t, output)
	require.Nil(t, output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, []string{"prepare", "input guard", "output guard", "finalize"}, runner.events)
}

// Confirms cancellation after provider work preserves usage but discards the response.
func TestProcessRunDiscardsOutputWhenCancelledDuringInference(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "retained"))
	defer cancel()
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		cancel()
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		finalize: func(finalizeCtx context.Context, output *task.Output, executionErr error) error {
			require.ErrorIs(t, executionErr, context.Canceled)
			require.NoError(t, finalizeCtx.Err())
			_, ok := finalizeCtx.Deadline()
			require.True(t, ok)
			require.Equal(t, "retained", finalizeCtx.Value(contextKey{}))
			require.Equal(t, task.OutcomeCancelled, output.Outcome)
			return nil
		},
	}

	output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, output)
	require.Nil(t, output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, task.OutcomeCancelled, output.Outcome)
	require.Equal(t, []string{"prepare", "input guard", "finalize"}, runner.events)
}

// Selects the provider's final-answer phase instead of returning commentary as task output.
func TestProcessRunUsesFinalAnswerPhase(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		response := processTestResponse()
		response.Output = prompts.Prompts{
			{Role: prompts.RoleAssistant, Phase: "commentary", Content: "commentary"},
			{Role: prompts.RoleAssistant, Phase: "final_answer", Content: "final"},
		}
		return response, nil
	}

	output, err := runProcessTest(t, context.Background(), inference, &processTestRunner{}, &schema.Output{})

	require.NoError(t, err)
	require.Equal(t, "final", output.Output)
}

// Rejects malformed structured output rather than returning invalid JSON as a result.
func TestProcessRunRejectsMalformedJSONOutput(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		response := processTestResponse()
		response.Output[0].Content = `{"answer":`
		return response, nil
	}
	runner := &processTestRunner{}
	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{Schema: &schema.Schema{MimeType: mime.ApplicationSchemaJSON}})

	require.ErrorIs(t, err, errors.ErrInvalidModelOutput)
	require.NotNil(t, output)
	require.Equal(t, `{"answer":`, output.Output)
	require.ErrorContains(t, err, "unexpected end of JSON input")
	require.Equal(t, uint64(1), output.Usage.Invocations)
}

// Exercises response selection and output formats through the full generation lifecycle.
func TestProcessRunCaptureResponse(t *testing.T) {
	file := &attachments.Attachment{Filename: "answer.txt", Data: []byte("attachment")}
	text := func(content string) prompts.Prompts {
		return prompts.Prompts{{Role: prompts.RoleAssistant, Content: content}}
	}
	tests := []struct {
		name        string
		messages    prompts.Prompts
		schema      *schema.Schema
		attachments attachments.Attachments
		model       string
		want        any
		wantMIME    mime.Type
		wantErr     error
	}{
		{
			name:    "no messages",
			wantErr: errors.ErrNoModelOutput,
		},
		{
			name: "ignored messages",
			messages: prompts.Prompts{
				nil,
				{
					Role:    prompts.RoleUser,
					Content: "user",
				},
				{
					Role:    prompts.RoleSystem,
					Content: "system",
				},
				{
					Role: prompts.RoleAssistant,
				},
				{
					Role:    prompts.RoleAssistant,
					Phase:   "commentary",
					Content: "thinking",
				},
				{
					Role:    prompts.RoleAssistant,
					Phase:   "unknown",
					Content: "unknown",
				},
			},
			wantErr: errors.ErrNoModelOutput,
		},
		{
			name:     "unphased text",
			messages: text("answer"),
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name:     "unknown MIME",
			messages: text("answer"),
			schema:   &schema.Schema{},
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name:     "plain text MIME",
			messages: text("answer"),
			schema: &schema.Schema{
				MimeType: mime.TextPlain,
			},
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name:     "other MIME",
			messages: text("<p>answer</p>"),
			schema: &schema.Schema{
				MimeType: mime.TextHTML,
			},
			want:     "<p>answer</p>",
			wantMIME: mime.TextHTML,
		},
		{
			name: "multiple unphased messages",
			messages: prompts.Prompts{
				nil,
				{
					Role:    prompts.RoleUser,
					Content: "ignore",
				},
				{
					Role:    prompts.RoleAssistant,
					Content: "first",
				},
				{
					Role: prompts.RoleAssistant,
				},
				{
					Role:    prompts.RoleAssistant,
					Phase:   "commentary",
					Content: "ignore",
				},
				{
					Role:    prompts.RoleAssistant,
					Content: "second",
				},
			},
			want:     "first\nsecond",
			wantMIME: mime.TextPlain,
		},
		{
			name: "final answers override unphased text",
			messages: prompts.Prompts{
				{
					Role:    prompts.RoleAssistant,
					Content: "fallback",
				},
				{
					Role:    prompts.RoleAssistant,
					Phase:   "final_answer",
					Content: "first",
				},
				{
					Role:    prompts.RoleAssistant,
					Phase:   "final_answer",
					Content: "second",
				},
			},
			want:     "first\nsecond",
			wantMIME: mime.TextPlain,
		},
		{
			name: "empty final answer uses fallback",
			messages: prompts.Prompts{
				{
					Role:  prompts.RoleAssistant,
					Phase: "final_answer",
				},
				{
					Role:    prompts.RoleAssistant,
					Content: "fallback",
				},
			},
			want:     "fallback",
			wantMIME: mime.TextPlain,
		},
		{
			name:     "whitespace remains text",
			messages: text(" \n"),
			want:     " \n",
			wantMIME: mime.TextPlain,
		},
		{
			name:     "resolved model",
			messages: text("answer"),
			model:    "resolved-model",
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name: "attachments only",
			attachments: attachments.Attachments{
				file,
			},
		},
		{
			name:     "attachments and text",
			messages: text("answer"),
			attachments: attachments.Attachments{
				file,
			},
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name: "attachments with ignored text",
			messages: prompts.Prompts{
				{
					Role:    prompts.RoleAssistant,
					Phase:   "commentary",
					Content: "thinking",
				},
			},
			attachments: attachments.Attachments{
				file,
			},
		},
		{
			name: "tool call type",
			messages: prompts.Prompts{
				{
					Role: prompts.RoleAssistant,
					Type: prompts.TypeToolCall,
				},
			},
			wantErr: errors.ErrCapabilityProviderRequired,
		},
		{
			name: "tool call payload",
			messages: prompts.Prompts{
				{
					Role:    prompts.RoleAssistant,
					Content: "not a final answer",
					ToolCalls: []prompts.ToolCall{
						{
							CallID: "call-1",
							Name:   "lookup",
						},
					},
				},
			},
			wantErr: errors.ErrCapabilityProviderRequired,
		},
		{
			name: "tool after final answer",
			messages: prompts.Prompts{
				{
					Role:    prompts.RoleAssistant,
					Phase:   "final_answer",
					Content: "not returned",
				},
				{
					Role: prompts.RoleAssistant,
					Type: prompts.TypeToolCall,
				},
			},
			wantErr: errors.ErrCapabilityProviderRequired,
		},
		{
			name:     "JSON object",
			messages: text(`{"answer":42}`),
			schema: &schema.Schema{
				MimeType: mime.ApplicationJSON,
			},
			want:     json.RawMessage(`{"answer":42}`),
			wantMIME: mime.ApplicationJSON,
		},
		{
			name:     "schema JSON",
			messages: text(`[1,2]`),
			schema: &schema.Schema{
				MimeType: mime.ApplicationSchemaJSON,
			},
			want:     json.RawMessage(`[1,2]`),
			wantMIME: mime.ApplicationSchemaJSON,
		},
		{
			name:     "JSON null",
			messages: text(`null`),
			schema: &schema.Schema{
				MimeType: mime.ApplicationJSON,
			},
			want:     json.RawMessage(`null`),
			wantMIME: mime.ApplicationJSON,
		},
		{
			name:     "malformed JSON",
			messages: text(`{"answer":`),
			schema: &schema.Schema{
				MimeType: mime.ApplicationJSON,
			},
			want:    `{"answer":`,
			wantErr: errors.ErrInvalidModelOutput,
		},
		{
			name:     "markdown fenced JSON",
			messages: text("```json\n{}\n```"),
			schema: &schema.Schema{
				MimeType: mime.ApplicationSchemaJSON,
			},
			want:    "```json\n{}\n```",
			wantErr: errors.ErrInvalidModelOutput,
		},
		{
			name:     "multiple JSON documents",
			messages: text(`{} {}`),
			schema: &schema.Schema{
				MimeType: mime.ApplicationJSON,
			},
			want:    `{} {}`,
			wantErr: errors.ErrInvalidModelOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: return the case's response from a mock provider; use real response processing.
			response := processTestResponse()
			response.Model = tt.model
			response.Output = tt.messages
			response.Attachments = tt.attachments

			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				return response, nil
			}

			// Prepare supplies model parameters; Finalize observes results without replacing them.
			parameters := params.New(map[string]any{"temperature": 0.5})
			runner := &processTestRunner{
				prepare: func(_ context.Context, prepared *task.Task) error {
					prepared.Model.Parameters = parameters
					return nil
				},
				// Finalize must see the same captured content and execution error as the caller.
				finalize: func(_ context.Context, output *task.Output, executionErr error) error {
					if tt.wantErr != nil {
						require.ErrorIs(t, executionErr, tt.wantErr)
					} else {
						require.NoError(t, executionErr)
					}
					require.Equal(t, tt.want, output.Output)
					return nil
				},
			}

			// Execute through Horizon.Run with the case's declared output schema.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{Schema: tt.schema})

			// Assert the returned error and outcome agree with the case's expected result.
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Equal(t, task.OutcomeFailed, output.Outcome)
			} else {
				require.NoError(t, err)
				require.Equal(t, task.OutcomeSucceeded, output.Outcome)
			}

			// Every case checks content, MIME type, and attachments, including empty results.
			require.Equal(t, tt.want, output.Output)
			require.Equal(t, tt.wantMIME, output.MimeType)
			require.Equal(t, tt.attachments, output.Attachments)

			// Prefer the provider's resolved model, falling back to the requested model.
			wantModel := tt.model
			if wantModel == "" {
				wantModel = "requested-model"
			}
			require.Equal(t, wantModel, output.Model.Slug)
			require.Equal(t, parameters, output.Model.Parameters)

			// Response-processing failures still retain the mock response's usage.
			require.Equal(t, uint64(1), output.Usage.Invocations)
			require.Equal(t, int64(18), output.Usage.TotalTokens)
			require.Equal(t, 0.25, output.Usage.APICost)

			// Hooks run in order, with exactly one generation and one finalization.
			require.Equal(t, []string{"prepare", "input guard", "output guard", "finalize"}, runner.events)
			inference.AssertCalled(t, mockprovider.Generate, 1)
		})
	}
}

// Generation preserves independent failures and never captures rejected or cancelled content.
func TestProcessRunGenerationFailures(t *testing.T) {
	providerErr := errors.New("provider failure")
	guardErr := errors.New("output guard failure")
	tests := []struct {
		name           string
		response       bool
		providerErr    error
		guardErr       error
		malformedJSON  bool
		cancelProvider bool
		cancelGuard    bool
		wantErrors     []error
		wantGuard      bool
	}{
		{
			name: "nil response",
			wantErrors: []error{
				errors.ErrNoModelOutput,
			},
		},
		{
			name:        "provider error without response",
			providerErr: providerErr,
			wantErrors: []error{
				providerErr,
			},
		},
		{
			name:        "provider and guard errors",
			response:    true,
			providerErr: providerErr,
			guardErr:    guardErr,
			wantErrors: []error{
				providerErr,
				guardErr,
			},
			wantGuard: true,
		},
		{
			name:          "provider and decoding errors",
			response:      true,
			providerErr:   providerErr,
			malformedJSON: true,
			wantErrors: []error{
				providerErr,
				errors.ErrInvalidModelOutput,
			},
			wantGuard: true,
		},
		{
			name:          "guard blocks malformed content",
			response:      true,
			guardErr:      guardErr,
			malformedJSON: true,
			wantErrors: []error{
				guardErr,
			},
			wantGuard: true,
		},
		{
			name:           "cancelled provider error with response",
			response:       true,
			providerErr:    providerErr,
			cancelProvider: true,
			wantErrors: []error{
				providerErr,
				context.Canceled,
			},
		},
		{
			name:           "cancelled provider error without response",
			providerErr:    providerErr,
			cancelProvider: true,
			wantErrors: []error{
				providerErr,
				context.Canceled,
			},
		},
		{
			name:           "cancelled nil response",
			cancelProvider: true,
			wantErrors: []error{
				context.Canceled,
			},
		},
		{
			name:        "cancelled output guard",
			response:    true,
			cancelGuard: true,
			wantErrors: []error{
				context.Canceled,
			},
			wantGuard: true,
		},
		{
			name:        "cancelled output guard and both errors",
			response:    true,
			providerErr: providerErr,
			guardErr:    guardErr,
			cancelGuard: true,
			wantErrors: []error{
				providerErr,
				guardErr,
				context.Canceled,
			},
			wantGuard: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: trigger failures and cancellation inside the provider or guard callbacks.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				if tt.cancelProvider {
					cancel()
				}
				if !tt.response {
					return nil, tt.providerErr
				}

				// Include attachments to detect content leaking past rejection or cancellation.
				response := processTestResponse()
				response.Attachments = attachments.Attachments{{Filename: "private.txt", Data: []byte("private")}}
				if tt.malformedJSON {
					response.Output[0].Content = `{"answer":`
				}
				return response, tt.providerErr
			}

			// The runner injects guard failures and checks cleanup without supplying output.
			runner := &processTestRunner{
				protectOutput: func(*provider.Response) error {
					if tt.cancelGuard {
						cancel()
					}
					return tt.guardErr
				},
				// Cleanup must remain usable after cancellation and receive every execution cause.
				finalize: func(finalizeCtx context.Context, output *task.Output, executionErr error) error {
					require.NoError(t, finalizeCtx.Err())
					for _, want := range tt.wantErrors {
						require.ErrorIs(t, executionErr, want)
					}
					if tt.malformedJSON && tt.guardErr == nil && !tt.cancelProvider && !tt.cancelGuard {
						require.Equal(t, `{"answer":`, output.Output)
					} else {
						require.Nil(t, output.Output)
					}
					return nil
				},
			}

			// Execute with JSON output enabled so malformed content reaches the real decoder check.
			output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{Schema: &schema.Schema{MimeType: mime.ApplicationJSON}})

			// Assert every independent cause survives wrapping and error joining.
			for _, want := range tt.wantErrors {
				require.ErrorIs(t, err, want)
			}

			// Guard rejection prevents decoding; a missing response must not mask other failures.
			if !tt.malformedJSON || tt.guardErr != nil {
				require.NotErrorIs(t, err, errors.ErrInvalidModelOutput)
			}
			if tt.name != "nil response" {
				require.NotErrorIs(t, err, errors.ErrNoModelOutput, "nil response must not mask a provider error or cancellation")
			}

			// Cancellation determines the outcome even when independent errors are also returned.
			wantOutcome := task.OutcomeFailed
			if tt.cancelProvider || tt.cancelGuard {
				wantOutcome = task.OutcomeCancelled
			}
			require.Equal(t, wantOutcome, output.Outcome)
			if tt.malformedJSON && tt.guardErr == nil && !tt.cancelProvider && !tt.cancelGuard {
				require.Equal(t, `{"answer":`, output.Output)
			} else {
				require.Nil(t, output.Output)
			}

			// Count the attempted call, but only account tokens and cost from a returned response.
			require.Equal(t, uint64(1), output.Usage.Invocations)
			if tt.response {
				require.Equal(t, int64(18), output.Usage.TotalTokens)
				require.Equal(t, 0.25, output.Usage.APICost)
			} else {
				require.Zero(t, output.Usage.TotalTokens)
				require.Zero(t, output.Usage.APICost)
			}

			// Rejected or cancelled responses must not expose their private attachments.
			if tt.guardErr != nil || tt.cancelProvider || tt.cancelGuard || !tt.response {
				require.Empty(t, output.Attachments)
			}

			// Skip the output guard when no response arrives or the provider cancels execution.
			// Every case must still finalize once, without retrying generation.
			wantEvents := []string{"prepare", "input guard"}
			if tt.wantGuard {
				wantEvents = append(wantEvents, "output guard")
			}
			wantEvents = append(wantEvents, "finalize")
			require.Equal(t, wantEvents, runner.events)
			inference.AssertCalled(t, mockprovider.Generate, 1)
		})
	}
}

// Checks the allow path with neither guard, either guard alone, or both guards.
// Unlike the rejection tests, this verifies that each optional interface is detected
// independently and that returning nil allows execution to continue. Mutations make
// the ordering observable: input changes reach inference, and output changes reach
// the caller (as they would for redaction).
func TestProcessRunOptionalGuards(t *testing.T) {
	for _, guards := range []string{"none", "input only", "output only", "both"} {
		t.Run(guards, func(t *testing.T) {
			// Setup: the provider mutates the request so that we can see that
			// nil-returning guards ran.
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				// Input-guard changes must reach the provider, not just the runner's local state.
				if guards == "input only" || guards == "both" {
					require.Equal(t, "guarded-model", request.Model)
				} else {
					require.Equal(t, "requested-model", request.Model)
				}
				return processTestResponse(), nil
			}

			// Both guards allow execution. Their mutations are markers proving they ran
			// before inference and response capture, not replacements for rejection checks.
			lifecycle := noopRunner()
			inputGuard := &mock.InputGuard{OnProtectInput: func(request *provider.Request) error {
				request.Model = "guarded-model"
				return nil
			}}
			outputGuard := &mock.OutputGuard{OnProtectOutput: func(response *provider.Response) error {
				response.Output[0].Content = "guarded answer"
				return nil
			}}

			// Embed only the selected guards so the runner's method set reflects each combination.
			var runner task.Runner = lifecycle
			switch guards {
			case "input only":
				runner = struct {
					*mock.Runner
					*mock.InputGuard
				}{lifecycle, inputGuard}
			case "output only":
				runner = struct {
					*mock.Runner
					*mock.OutputGuard
				}{lifecycle, outputGuard}
			case "both":
				runner = struct {
					*mock.Runner
					*mock.InputGuard
					*mock.OutputGuard
				}{lifecycle, inputGuard, outputGuard}
			}

			// Execute the same task with only this case's selected interfaces available.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

			// Assert execution succeeds for every combination, including no guards.
			require.NoError(t, err)

			// Output-guard changes must reach the result; absent guards must never be called.
			want := "answer"
			if guards == "output only" || guards == "both" {
				want = "guarded answer"
				outputGuard.AssertCalled(t, mock.ProtectOutput, 1)
			} else {
				outputGuard.AssertNotCalled(t, mock.ProtectOutput)
			}
			if guards == "input only" || guards == "both" {
				inputGuard.AssertCalled(t, mock.ProtectInput, 1)
			} else {
				inputGuard.AssertNotCalled(t, mock.ProtectInput)
			}
			require.Equal(t, want, output.Output)
			require.Equal(t, task.OutcomeSucceeded, output.Outcome)

			// Guard combinations must not change the single-generation, single-finalization lifecycle.
			lifecycle.AssertCalled(t, mock.Finalize, 1)
			inference.AssertCalled(t, mockprovider.Generate, 1)
		})
	}
}

// Validates required execution inputs before invoking runner lifecycle hooks.
func TestProcessRunRequiresTaskRunnerAndOutput(t *testing.T) {
	tests := []struct {
		name string
		task *task.Task
		run  task.Runner
		want error
	}{
		{
			name: "task",
			run:  &processTestRunner{},
			want: errors.ErrTaskRequired,
		},
		{
			name: "runner",
			task: &task.Task{
				Output: &schema.Output{},
			},
			want: errors.ErrRunnerRequired,
		},
		{
			name: "output",
			task: &task.Task{},
			run:  &processTestRunner{},
			want: errors.ErrTaskOutputRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := horizon.Run(context.Background(), nil, test.task, test.run)
			require.ErrorIs(t, err, test.want)
		})
	}
}

// Confirms that the task config constructs a provider through the public API.
func TestProcessPrepareBuildsConfiguredProvider(t *testing.T) {
	config := inferenceConfig(t)
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return &processTestRunner{}, nil }))
	output, err := h.Run(context.Background(), nil, &task.Task{Provider: &config, Output: &schema.Output{}})
	require.NoError(t, err)
	require.Equal(t, "generated", output.Output)
}

// Reports missing provider configuration as a setup error.
func TestProcessPrepareRequiresProviderConfiguration(t *testing.T) {
	_, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, &processTestRunner{})
	require.ErrorIs(t, err, errors.ErrProviderRequired)
}

// Concurrent executions keep prepared tasks, requests, guards, and outputs isolated.
func TestProcessConcurrentIsolation(t *testing.T) {
	const runs = 12
	type executionKey struct{}

	config := testProviderConfig()
	inference := mockprovider.New(config.ID)
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		return &provider.Response{
			Model: request.Model,
			Output: prompts.Prompts{{
				Role:    prompts.RoleAssistant,
				Content: request.Model,
			}},
		}, nil
	}

	original := &task.Task{
		Provider: &config,
		Model:    task.Model{Slug: "shared-model"},
		Output:   &schema.Output{},
	}
	h := newTestHorizon(t, testFactory(func(ctx context.Context) (task.Runner, error) {
		executionID, ok := ctx.Value(executionKey{}).(string)
		if !ok {
			return nil, fmt.Errorf("execution ID missing from runner context")
		}
		runner := &processTestRunner{}
		runner.prepare = func(_ context.Context, prepared *task.Task) error {
			if prepared.Provider == original.Provider {
				return fmt.Errorf("execution reused the original provider config")
			}
			prepared.Model.Slug = executionID
			return nil
		}
		runner.protectInput = func(request *provider.Request) error {
			if request.Model != executionID {
				return fmt.Errorf("input guard got model %q, want %q", request.Model, executionID)
			}
			return nil
		}
		runner.protectOutput = func(response *provider.Response) error {
			if response.Output[0].Content != executionID {
				return fmt.Errorf("output guard got response %q, want %q", response.Output[0].Content, executionID)
			}
			return nil
		}
		return runner, nil
	}))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)

	type result struct {
		id     string
		output *task.Output
		err    error
	}
	results := make(chan result, runs)
	for i := range runs {
		id := fmt.Sprintf("run-%d", i)
		go func() {
			ctx := context.WithValue(context.Background(), executionKey{}, id)
			output, err := h.Run(ctx, &task.Input{}, original)
			results <- result{id: id, output: output, err: err}
		}()
	}

	for range runs {
		got := <-results
		require.NoError(t, got.err)
		require.Equal(t, got.id, got.output.Output)
		require.Equal(t, got.id, got.output.Model.Slug)
		require.Equal(t, task.OutcomeSucceeded, got.output.Outcome)
	}
	require.Equal(t, "shared-model", original.Model.Slug)
	require.Same(t, &config, original.Provider)
}

// Holds configurable host hooks for exercising one execution lifecycle.
type processTestRunner struct {
	events        []string
	prepare       func(context.Context, *task.Task) error
	prepareErr    error
	finalizeErr   error
	protectInput  func(*provider.Request) error
	protectOutput func(*provider.Response) error
	finalize      func(context.Context, *task.Output, error) error
}

func (r *processTestRunner) Prepare(ctx context.Context, prepared *task.Task) error {
	r.events = append(r.events, "prepare")
	if r.prepare != nil {
		if err := r.prepare(ctx, prepared); err != nil {
			return err
		}
	}
	return r.prepareErr
}

func (r *processTestRunner) Finalize(ctx context.Context, output *task.Output, executionErr error) error {
	r.events = append(r.events, "finalize")
	if r.finalize != nil {
		if err := r.finalize(ctx, output, executionErr); err != nil {
			return err
		}
	}
	return r.finalizeErr
}

func (r *processTestRunner) ProtectInput(request *provider.Request) error {
	r.events = append(r.events, "input guard")
	if r.protectInput != nil {
		return r.protectInput(request)
	}
	return nil
}

func (r *processTestRunner) ProtectOutput(response *provider.Response) error {
	r.events = append(r.events, "output guard")
	if r.protectOutput != nil {
		return r.protectOutput(response)
	}
	return nil
}

func runProcessTest(t *testing.T, ctx context.Context, inference provider.Provider, runner task.Runner, output *schema.Output) (*task.Output, error) {
	t.Helper()
	config := testProviderConfig()
	config.ID = inference.ID()
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return runner, nil }))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)
	return h.Run(ctx, &task.Input{}, &task.Task{
		Provider: &config,
		Model:    task.Model{Slug: "requested-model"},
		Output:   output,
	})
}

func processTestResponse() *provider.Response {
	return &provider.Response{
		Model: "served-model",
		Usage: provider.Usage{
			InputTokens:  11,
			OutputTokens: 7,
			TotalTokens:  18,
			APICost:      0.25,
		},
		Output: prompts.Prompts{{
			Role:    prompts.RoleAssistant,
			Type:    prompts.TypeMessage,
			Content: "answer",
		}},
	}
}
