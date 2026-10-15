package horizon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/actions"
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
	"go.rtnl.ai/x/rlog"
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
	require.Equal(t, "answer", output.Output)
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

// Output guards receive validation snapshots. Their edits must affect neither
// delivered content, provider-owned data, accounting, nor recorded debug payloads.
func TestProcessOutputGuardMutationsIgnored(t *testing.T) {
	response := processTestResponse()
	response.Output[0].Type = prompts.TypeMessage
	response.Output[0].Meta = map[string]any{"nested": []any{"original"}}
	response.Meta = provider.Meta{"nested": map[string]any{"value": "original"}}
	response.Attachments = attachments.Attachments{{Filename: "file.txt", Data: []byte("abc")}}
	original := response.Clone()
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return response, nil
	}
	runner := &processTestRunner{protectOutput: func(snapshot *provider.Response) error {
		require.NotSame(t, response, snapshot)
		require.NotSame(t, response.Output[0], snapshot.Output[0])
		snapshot.Output[0].Content = "changed"
		snapshot.Output[0].Meta["nested"].([]any)[0] = "changed"
		snapshot.Output[0].ToolCalls = []prompts.ToolCall{{CallID: "unexpected", Name: "lookup"}}
		snapshot.Attachments[0].Filename = "changed"
		snapshot.Attachments[0].Data[0] = 'X'
		snapshot.Model = "changed"
		snapshot.Usage.TotalTokens = 999
		snapshot.Meta["nested"].(map[string]any)["value"] = "changed"
		return nil
	}}

	// An accepted response remains the provider's original answer, not the edits.
	output, err := runProcessTest(t, t.Context(), inference, runner, &schema.Output{})
	require.NoError(t, err)
	require.Equal(t, original, response)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, original.Model, output.Model.Slug)
	require.Equal(t, original.Usage.TotalTokens, output.Usage.TotalTokens)
	require.Equal(t, original.Attachments, output.Attachments)
	messages := output.Messages()
	require.Equal(t, original.Output[0], messages[len(messages)-1])

	// The raw response and delivered message are distinct events with one identity
	// for their relationship. Both must preserve the original nested content.
	var raw, delivered *actions.Action
	for _, entry := range output.Actions.Entries() {
		if entry.Type == actions.TypeGenerationResponse {
			raw = entry
		}
		if entry.Type == actions.TypeMessage && entry.Sender == actions.EntityModel {
			delivered = entry
		}
	}
	require.NotNil(t, raw)
	require.NotNil(t, delivered)
	require.Equal(t, raw.CorrelationID, delivered.CorrelationID)
	require.Equal(t, original.Output, raw.GenerationResponse.Output)
	require.Equal(t, original.Output, delivered.Messages)
	require.Equal(t, raw.Attachments, delivered.Attachments)
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
			name: "reasoning is not final output",
			messages: prompts.Prompts{
				{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Content: "private reasoning"},
				{Role: prompts.RoleAssistant, Type: prompts.TypeMessage, Content: "answer"},
			},
			want:     "answer",
			wantMIME: mime.TextPlain,
		},
		{
			name: "reasoning alone is not final output",
			messages: prompts.Prompts{
				{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Content: "private reasoning"},
			},
			wantErr: errors.ErrNoModelOutput,
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
			wantMessages := append(prompts.Prompts{}, tt.messages...)
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
					require.Equal(t, wantMessages, output.Messages())
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

			// History is independent of final-answer decoding and retains every unfiltered message.
			require.Equal(t, wantMessages, output.Messages())

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

			// Response events record only what the provider returned, not errors
			// from cancellation, guards, or Horizon's missing-output validation.
			var responseEvent *actions.Action
			for _, event := range output.Actions.Entries() {
				if event.Type == actions.TypeGenerationResponse {
					responseEvent = event
					break
				}
			}
			require.NotNil(t, responseEvent)
			require.Equal(t, tt.response, responseEvent.GenerationResponse != nil)
			if tt.providerErr != nil {
				require.Equal(t, tt.providerErr.Error(), responseEvent.Error)
			} else {
				require.Empty(t, responseEvent.Error)
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

			// Rejected or cancelled responses must not expose content through attachments or history.
			if tt.guardErr != nil || tt.cancelProvider || tt.cancelGuard || !tt.response {
				require.Empty(t, output.Attachments)
				require.Empty(t, output.Messages())
			} else {
				require.Len(t, output.Messages(), 1)
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
// Both input- and output-guard mutations stay isolated from execution.
func TestProcessRunOptionalGuards(t *testing.T) {
	for _, guards := range []string{"none", "input only", "output only", "both"} {
		t.Run(guards, func(t *testing.T) {
			// Setup: run each guard combination with a request marker and final response.
			// The input guard edits only its validation copy; the output guard redacts text.
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				// Input-guard changes are intentionally absent from the provider request.
				require.Equal(t, "requested-model", request.Model)
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

			// Guard mutations are ignored; absent guards must never be called.
			want := "answer"
			if guards == "output only" || guards == "both" {
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

// TestProcessOutputMessages verifies returned history retains reasoning and unknown types by default.
func TestProcessOutputMessages(t *testing.T) {
	// Setup: return reasoning, an unknown type, and a final answer in one response.
	input := &prompts.Prompt{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question"}
	answer := &prompts.Prompt{
		Role:    prompts.RoleAssistant,
		Type:    prompts.TypeMessage,
		Content: "answer",
	}
	response := processTestResponse()
	response.Output = prompts.Prompts{
		{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Content: "private reasoning"},
		{Role: prompts.RoleAssistant, Type: prompts.TypeUnknown, ID: "unknown-item"},
		answer,
	}
	renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
		return prompts.Prompts{input}, nil
	}}
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		require.Equal(t, prompts.Prompts{input}, request.Input)
		return response, nil
	}
	runner := &processTestRunner{
		protectInput: func(request *provider.Request) error {
			require.Equal(t, "question", request.Input[0].Content)
			request.Input[0].Content = "guard mutation"
			return nil
		},
		finalize: func(_ context.Context, output *task.Output, executionErr error) error {
			require.NoError(t, executionErr)
			require.Equal(t, prompts.Prompts{input, response.Output[0], response.Output[1], answer}, output.Messages())
			return nil
		},
	}
	host := struct {
		*processTestRunner
		*mock.Renderer
	}{runner, renderer}

	// Execute the normal single-generation lifecycle, including Finalize.
	output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{})

	// The final answer remains assistant text while the full transcript is returned by default.
	require.NoError(t, err)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, prompts.Prompts{input, response.Output[0], response.Output[1], answer}, output.Messages())
	require.Len(t, response.Output, 3)
	require.Equal(t, prompts.TypeReasoning, response.Output[0].Type)
	require.Equal(t, prompts.TypeUnknown, response.Output[1].Type)

	// The conversation is an independent snapshot. Editing a returned slice or
	// prompt must not change either the cached transcript or provider response.
	messages := output.Messages()
	require.NotSame(t, input, messages[0])
	require.NotSame(t, answer, messages[3])
	messages[0].Content = "changed question"
	messages[3] = &prompts.Prompt{Content: "replacement"}
	require.Equal(t, "question", output.Messages()[0].Content)
	require.Equal(t, "answer", output.Messages()[3].Content)
	require.Same(t, answer, response.Output[2])
	inference.AssertCalled(t, mockprovider.Generate, 1)
}

// Callers can filter output messages without changing the recorded conversation.
func TestProcessOutputMessageFilters(t *testing.T) {
	input := &prompts.Prompt{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question"}
	answer := &prompts.Prompt{Role: prompts.RoleAssistant, Type: prompts.TypeMessage, Content: "answer"}
	response := processTestResponse()
	response.Output = prompts.Prompts{
		answer,
		{Role: prompts.RoleAssistant, Type: prompts.TypeRefusal},
		{Role: prompts.RoleDeveloper, Type: prompts.TypeMessage, Content: "developer note"},
	}
	renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
		return prompts.Prompts{input}, nil
	}}
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return response, nil
	}
	runner := &processTestRunner{}
	host := struct {
		*processTestRunner
		*mock.Renderer
	}{runner, renderer}

	output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{})

	require.NoError(t, err)
	filtered := output.Messages().FilterOutTypes(prompts.TypeRefusal).FilterOutRoles(prompts.RoleDeveloper)
	require.Equal(t, prompts.Prompts{input, answer}, filtered)
	require.Equal(t, prompts.Prompts{input, answer, response.Output[1], response.Output[2]}, output.Messages())
}

// Attachment preparation errors stop before inference, including cancellation
// between files and processors returning unusable data.
func TestProcessAttachmentFailures(t *testing.T) {
	processorErr := errors.New("attachment processor failed")
	tests := []struct {
		name            string
		files           attachments.Attachments
		processorError  bool
		processorNil    bool
		cancelProcessor bool
		downloadFailure bool
		want            error
	}{
		{
			name:  "nil attachment",
			files: attachments.Attachments{nil},
			want:  errors.ErrNilAttachment,
		},
		{
			name:           "processor failure",
			processorError: true,
			want:           processorErr,
		},
		{
			name:         "processor returns nil",
			processorNil: true,
			want:         errors.ErrNilAttachment,
		},
		{
			name:            "cancellation between attachments",
			cancelProcessor: true,
			want:            context.Canceled,
		},
		{
			name:            "download failure",
			downloadFailure: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: local failures and cancellation, with no external HTTP requests.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			files := tt.files
			if files == nil {
				files = attachments.Attachments{{Filename: "file.txt", Data: []byte("data")}}
			}
			if tt.cancelProcessor {
				files = append(files, &attachments.Attachment{Filename: "second.txt", Data: []byte("data")})
			}
			if tt.downloadFailure {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
				defer server.Close()
				files = attachments.Attachments{{URL: server.URL}}
			}
			lifecycle := noopRunner()
			var runner task.Runner = lifecycle
			if tt.processorError || tt.processorNil || tt.cancelProcessor {
				processor := &mock.AttachmentProcessor{OnProcessAttachment: func(file *attachments.Attachment) (*attachments.Attachment, error) {
					if tt.processorError {
						return nil, processorErr
					}
					if tt.processorNil {
						return nil, nil
					}
					cancel()
					return file, nil
				}}
				runner = struct {
					*mock.Runner
					*mock.AttachmentProcessor
				}{lifecycle, processor}
			}
			inference := mockprovider.New(ulid.Make())

			// Execute preparation and cleanup without reaching generation.
			output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{}, &task.Input{Attachments: files})

			// Assert the failure and absence of provider calls; Finalize still runs once.
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
			require.Zero(t, output.Usage.Invocations)
			inference.AssertNotCalled(t, mockprovider.Generate)
			lifecycle.AssertCalled(t, mock.Finalize, 1)
		})
	}
}

// Interaction events preserve raw provider output for debugging, while the
// conversation contains only accepted content. Every completed provider call
// must have a response event, even if a guard rejects it or delivery is cancelled.
func TestProcessActionLog(t *testing.T) {
	guardErr := errors.New("blocked")
	for _, test := range []struct {
		name           string
		inputRejected  bool
		outputRejected bool
		cancelled      bool
		wantEvents     int
	}{
		{
			name:       "accepted with ignored mutations",
			wantEvents: 6,
		},
		{
			name:          "input rejected",
			inputRejected: true,
			wantEvents:    2,
		},
		{
			name:           "output rejected",
			outputRejected: true,
			wantEvents:     5,
		},
		{
			name:       "cancelled delivery",
			cancelled:  true,
			wantEvents: 4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			response := processTestResponse()
			response.Output = prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeMessage, Content: "raw"}}
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				if test.cancelled {
					cancel()
				}
				return response, nil
			}
			runner := &processTestRunner{
				protectInput: func(*provider.Request) error {
					if test.inputRejected {
						return guardErr
					}
					return nil
				},
				protectOutput: func(response *provider.Response) error {
					response.Output[0].Content = "guarded"
					if test.outputRejected {
						return guardErr
					}
					return nil
				},
			}
			renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
				return prompts.Prompts{{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question"}}, nil
			}}
			host := struct {
				*processTestRunner
				*mock.Renderer
			}{runner, renderer}

			// Execute through the actual process so recording order and acceptance
			// are tested at the provider/guard boundaries, not just via helpers.
			output, err := runProcessTest(t, ctx, inference, host, &schema.Output{})
			if test.cancelled {
				require.ErrorIs(t, err, context.Canceled)
			} else if test.inputRejected || test.outputRejected {
				require.ErrorIs(t, err, guardErr)
			} else {
				require.NoError(t, err)
			}
			entries := output.Actions.Entries()
			require.Len(t, entries, test.wantEvents)
			require.Equal(t, actions.TypeMessage, entries[0].Type)
			require.Equal(t, actions.TypeInputGuard, entries[1].Type)
			if test.inputRejected {
				require.NotEmpty(t, entries[1].Error)
				require.Len(t, output.Messages(), 1)
				return
			}
			require.Equal(t, actions.TypeGenerationRequest, entries[2].Type)
			require.Equal(t, actions.TypeGenerationResponse, entries[3].Type)
			require.Equal(t, entries[2].CorrelationID, entries[3].CorrelationID)
			require.Equal(t, "raw", entries[3].GenerationResponse.Output[0].Content)
			require.Empty(t, entries[3].Error)
			if test.cancelled {
				require.Len(t, output.Messages(), 1)
				return
			}
			require.Equal(t, actions.TypeOutputGuard, entries[4].Type)
			require.Equal(t, entries[3].CorrelationID, entries[4].CorrelationID)
			if test.outputRejected {
				require.NotEmpty(t, entries[4].Error)
				require.Len(t, output.Messages(), 1)
			} else {
				require.Equal(t, actions.TypeMessage, entries[5].Type)
				require.Equal(t, entries[3].CorrelationID, entries[5].CorrelationID)
				require.Equal(t, "raw", entries[5].Messages[0].Content)
				require.Len(t, output.Messages(), 2)
				require.Equal(t, "raw", output.Messages()[1].Content)
			}
		})
	}
}

// Compare complete runs with and without action recording. Each run exercises
// every runner interface, executes one capability, and makes two model requests.
// Requests and responses include schemas, attachments, metadata, and tool content
// so cloning costs are measured with populated payloads, not empty containers.
// The nil-log baseline still constructs recording arguments and emits warnings;
// it is not a pre-log build.
func BenchmarkProcessActions(b *testing.B) {
	// Format warnings normally but discard their bytes so terminal I/O does not
	// dominate the nil-log case. Restore the logger for other tests/benchmarks.
	previous := slog.Default()
	rlog.SetDefault(rlog.New(slog.New(slog.NewTextHandler(io.Discard, nil))))
	b.Cleanup(func() { rlog.SetDefault(rlog.New(previous)) })

	// Record with and without actions logging.
	for _, record := range []bool{false, true} {
		name := "WithoutActions"
		if record {
			name = "WithActions"
		}
		b.Run(name, func(b *testing.B) {
			// Reuse fixtures and the provider cache. Each iteration creates a fresh
			// process, requests a tool, delivers its result, and gets a final answer.
			messages := prompts.Prompts{
				{Role: prompts.RoleSystem, Type: prompts.TypeMessage, Content: "Answer the user's question."},
				{
					Role: prompts.RoleUser, Type: prompts.TypeMessage,
					Content: strings.Repeat("question ", 128),
					Citations: []prompts.Citation{{
						StartIndex: 0, EndIndex: 8,
						Title: "Input context", URL: "https://example.com/context",
					}},
					Meta: map[string]any{
						"source": map[string]any{"name": "benchmark"},
						"tags":   []any{"question"},
					},
				},
			}
			tools := []capabilities.Definition{{
				Name: "lookup", Description: "Look up supporting context.",
				InputSchema: &schema.Schema{
					Name: "lookup-input", MimeType: mime.ApplicationSchemaJSON,
					Data: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
				},
			}}
			call := prompts.ToolCall{
				CallID: "benchmark-call", Name: "lookup", Arguments: `{"query":"benchmark"}`,
			}
			toolResult := strings.Repeat("lookup result ", 128)
			created := time.Now().UTC()

			// Both provider responses carry files and metadata. The first requests
			// a tool; the second returns cited text matching the output schema.
			toolResponse := &provider.Response{
				ID: "benchmark-tool-response", Model: "benchmark-model", Created: created,
				Usage: provider.Usage{InputTokens: 300, OutputTokens: 100, TotalTokens: 400, APICost: 0.001},
				Output: prompts.Prompts{
					{
						Role: prompts.RoleAssistant, Type: prompts.TypeReasoning,
						Content: strings.Repeat("reasoning ", 64),
						Meta:    map[string]any{"source": "model"},
					},
					{
						Role: prompts.RoleAssistant, Type: prompts.TypeToolCall,
						ToolCalls: []prompts.ToolCall{call},
					},
				},
				Attachments: attachments.Attachments{{
					Filename: "plan.txt", ContentType: "text/plain",
					Data: []byte("Look up supporting context."),
				}},
				Meta: provider.Meta{"request": map[string]any{"id": "benchmark-tool"}},
			}
			answer := `{"answer":"` + strings.Repeat("answer ", 128) + `"}`
			finalResponse := &provider.Response{
				ID: "benchmark-final-response", Model: "benchmark-model", Created: created,
				Usage: provider.Usage{InputTokens: 600, OutputTokens: 250, TotalTokens: 850, APICost: 0.002},
				Output: prompts.Prompts{{
					Role: prompts.RoleAssistant, Type: prompts.TypeMessage,
					Content: answer, Status: "completed", Phase: "final_answer",
					Citations: []prompts.Citation{{
						StartIndex: 11, EndIndex: 17,
						Title: "Lookup result", URL: "https://example.com/result",
					}},
					Meta: map[string]any{
						"source": map[string]any{"name": "lookup"},
						"tags":   []string{"answer"},
					},
				}},
				Attachments: attachments.Attachments{{
					Filename: "answer.json", ContentType: "application/json", Data: []byte(answer),
				}},
				Meta: provider.Meta{"request": map[string]any{"id": "benchmark-final"}},
			}
			runner := &benchmarkActionsRunner{
				Runner: &mock.Runner{OnFinalize: func(context.Context, *task.Output, error) error { return nil }},

				// Optional processors leave their inputs unchanged.
				InputProcessor: &mock.InputProcessor{OnProcessInput: func(input *task.Input) (*task.Input, error) {
					return input, nil
				}},
				ContextProcessor: &mock.ContextProcessor{OnProcessContext: func(ctx prompts.Context) (prompts.Context, error) {
					return ctx, nil
				}},
				AttachmentProcessor: &mock.AttachmentProcessor{OnProcessAttachment: func(file *attachments.Attachment) (*attachments.Attachment, error) {
					return file, nil
				}},
				Renderer: &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
					return messages, nil
				}},
				InputGuard:  &mock.InputGuard{OnProtectInput: func(*provider.Request) error { return nil }},
				OutputGuard: &mock.OutputGuard{OnProtectOutput: func(*provider.Response) error { return nil }},

				// Resolve and execute a real mock tool exchange on every run.
				CapabilityRunner: &mock.CapabilityRunner{
					OnLookupCapabilities: func(context.Context, []capabilities.Name) ([]capabilities.Definition, error) {
						return tools, nil
					},
					OnExecuteCapability: func(context.Context, capabilities.Request) (capabilities.Response, *capabilities.Error) {
						return capabilities.NewToolTextContentResponse(toolResult), nil
					},
				},
			}
			h, err := horizon.New(testFactory(func(context.Context) (task.Runner, error) { return runner, nil }))
			require.NoError(b, err)
			configuration := testProviderConfig()
			inference := mockprovider.New(configuration.ID)
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				// Respond to the delivered tool result rather than keeping a turn
				// counter that would need resetting for every benchmark iteration.
				last := request.Input[len(request.Input)-1]
				if len(last.ToolResults) > 0 {
					return finalResponse, nil
				}
				return toolResponse, nil
			}
			_, err = h.GetOrCreateProvider(configuration, inference)
			require.NoError(b, err)

			definition := &task.Task{
				Provider: &configuration,
				Model: task.Model{Slug: "benchmark-model", Parameters: params.New(map[string]any{
					params.Temperature:  0.5,
					params.MaxToolTurns: 1,
				})},
				Capabilities: []capabilities.Name{"lookup"},
				Output: &schema.Output{Schema: &schema.Schema{
					Name: "answer", Description: "The final answer.", Strict: true,
					MimeType: mime.ApplicationSchemaJSON,
					Data:     `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
				}},
			}
			input := &task.Input{
				Context: prompts.Context{"query": "benchmark"},
				Attachments: attachments.Attachments{{
					Filename: "context.txt", ContentType: "text/plain",
					Data: []byte(strings.Repeat("data", 1024)),
				}},
			}
			ctx := b.Context()
			run := func() (*task.Output, error) {
				proc := h.NewProcessForTest(runner, input, definition)
				runner.OnPrepare = func(context.Context, *task.Task) error {
					if !record {
						proc.SetActionsForTest(nil)
					}
					return nil
				}
				return proc.Run(ctx)
			}

			// Check every hook and the complete exchange before starting the timer.
			output, err := run()
			require.NoError(b, err)
			require.Equal(b, json.RawMessage(answer), output.Output)
			require.Equal(b, uint64(2), output.Usage.Invocations)
			require.Equal(b, uint64(1), output.Usage.ToolCalls)
			require.Len(b, output.Attachments, 2)

			runner.Runner.AssertCalled(b, mock.Prepare, 1)
			runner.Runner.AssertCalled(b, mock.Finalize, 1)
			runner.InputProcessor.AssertCalled(b, mock.ProcessInput, 1)
			runner.ContextProcessor.AssertCalled(b, mock.ProcessContext, 1)
			runner.AttachmentProcessor.AssertCalled(b, mock.ProcessAttachment, 1)
			runner.Renderer.AssertCalled(b, mock.Render, 1)
			runner.InputGuard.AssertCalled(b, mock.ProtectInput, 2)
			runner.OutputGuard.AssertCalled(b, mock.ProtectOutput, 2)
			runner.CapabilityRunner.AssertCalled(b, mock.LookupCapabilities, 1)
			runner.CapabilityRunner.AssertCalled(b, mock.ExecuteCapability, 1)
			inference.AssertCalled(b, mockprovider.Generate, 2)

			if record {
				require.Equal(b, 14, output.Actions.Len())
				require.Equal(b, prompts.Prompts{
					messages[0], messages[1], toolResponse.Output[0], toolResponse.Output[1],
					{
						Role: prompts.RoleTool, Type: prompts.TypeMessage,
						ToolResults: []prompts.ToolResponse{{CallID: call.CallID, Content: toolResult}},
					},
					finalResponse.Output[0],
				}, output.Messages())

				// Ensure the larger request/response fields are actually recorded.
				entries := output.Actions.Entries()
				require.Equal(b, tools, entries[2].GenerationRequest.Tools)
				require.Equal(b, definition.Output.Schema, entries[2].GenerationRequest.OutputSchema)
				require.NotNil(b, entries[2].GenerationRequest.Params)
				require.Len(b, entries[2].Attachments, 1)
				require.Equal(b, toolResponse.Meta, entries[3].GenerationResponse.Meta)
				require.Len(b, entries[3].Attachments, 1)
				require.Len(b, entries[11].Attachments, 1)
			} else {
				require.Nil(b, output.Actions)
			}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
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

// Runs an execution with a cached mock provider and optional application input.
func runProcessTest(t *testing.T, ctx context.Context, inference provider.Provider, runner task.Runner, output *schema.Output, inputs ...*task.Input) (*task.Output, error) {
	t.Helper()
	config := testProviderConfig()
	config.ID = inference.ID()
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return runner, nil }))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)
	input := &task.Input{}
	if len(inputs) > 0 {
		input = inputs[0]
	}
	return h.Run(ctx, input, &task.Task{
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

// Ensure the benchmark runner implements the complete set of runner interfaces.
var _ task.CompleteRunner = (*benchmarkActionsRunner)(nil)

// Compose the existing mocks so both cases expose the same runner interfaces.
type benchmarkActionsRunner struct {
	*mock.Runner
	*mock.InputProcessor
	*mock.ContextProcessor
	*mock.AttachmentProcessor
	*mock.Renderer
	*mock.InputGuard
	*mock.OutputGuard
	*mock.CapabilityRunner
}
