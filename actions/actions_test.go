package actions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/rlog"
)

// Recording copies the input. Editing that input or anything returned by Entries
// or Messages must not change the log.
func TestSnapshots(t *testing.T) {
	// Setup test data.
	input := message(prompts.RoleUser, "original")
	input.Meta = map[string]any{"nested": map[string]any{"value": "original"}}

	request := &provider.Request{
		Input:        prompts.Prompts{input},
		Params:       params.New(map[string]any{"temperature": 0.5}),
		OutputSchema: &schema.Schema{Name: "output"},
		Tools: []capabilities.Definition{{
			Name:        "lookup",
			InputSchema: &schema.Schema{Name: "lookup-input"},
		}},
	}
	response := &provider.Response{
		Output: prompts.Prompts{message(prompts.RoleAssistant, "answer")},
		Meta:   provider.Meta{"tag": "original"},
	}
	var log actions.Actions

	correlation := log.AddGenerationRequest(t.Context(), request)
	log.AddGenerationResponse(t.Context(), correlation, response, nil)
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, request.Input)
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner, response.Output, actions.WithCorrelation(correlation))

	// Mutations after recording cannot rewrite historical payloads.
	input.Content = "changed"
	input.Meta["nested"].(map[string]any)["value"] = "changed"
	request.Params.Set("temperature", 1.0)

	request.OutputSchema.Name = "changed"
	request.Tools[0].InputSchema.Name = "changed"
	response.Output[0].Content = "changed"
	response.Meta["tag"] = "changed"

	entries := log.Entries()
	require.Equal(t, "original", entries[0].GenerationRequest.Input[0].Content)
	require.Equal(t, "original", entries[0].GenerationRequest.Input[0].Meta["nested"].(map[string]any)["value"])
	value, ok := entries[0].GenerationRequest.Params.Get("temperature")
	require.True(t, ok)
	require.Equal(t, 0.5, value)

	require.Equal(t, "output", entries[0].GenerationRequest.OutputSchema.Name)
	require.Equal(t, "lookup-input", entries[0].GenerationRequest.Tools[0].InputSchema.Name)
	require.Equal(t, "original", entries[1].GenerationResponse.Meta["tag"])

	// Editing returned entries must not change later reads.
	entries[0].GenerationRequest.Input[0].Content = "entry mutation"
	entries[0].GenerationRequest.Params.Set("temperature", 0.75)
	entries[0].GenerationRequest.OutputSchema.Name = "entry mutation"
	entries[0].GenerationRequest.Tools[0].InputSchema.Name = "entry mutation"
	entries[2].Messages[0].Content = "entry mutation"
	entries[2].Messages[0].Meta["nested"].(map[string]any)["value"] = "entry mutation"

	fresh := log.Entries()[0].GenerationRequest
	value, ok = fresh.Params.Get("temperature")
	require.True(t, ok)
	require.Equal(t, 0.5, value)
	require.Equal(t, "output", fresh.OutputSchema.Name)
	require.Equal(t, "lookup-input", fresh.Tools[0].InputSchema.Name)

	// The returned conversation is also safe to edit.
	transcript := log.Messages()
	require.Equal(t, "original", transcript[0].Content)
	require.Equal(t, "answer", transcript[1].Content)

	transcript[0].Content = "transcript mutation"
	transcript[0].Meta["nested"].(map[string]any)["value"] = "transcript mutation"
	transcript[1] = message(prompts.RoleAssistant, "replacement")
	require.Equal(t, "answer", log.Messages()[1].Content)
	require.Equal(t, "original", log.Messages()[0].Meta["nested"].(map[string]any)["value"])
	require.Equal(t, "original", log.Entries()[2].Messages[0].Content)

	// Appending another message must not extend a conversation already returned.
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner,
		prompts.Prompts{message(prompts.RoleUser, "another question")})
	require.Len(t, transcript, 2)
	require.Len(t, log.Messages(), 3)
}

// Invalid event types and nil events are skipped without advancing the sequence.
func TestInvalidActions(t *testing.T) {
	var log actions.Actions
	require.Zero(t, log.Add(t.Context(), nil))
	require.Zero(t, log.Add(t.Context(), &actions.Action{Type: actions.TypeUnknown}))
	require.Zero(t, log.Add(t.Context(), &actions.Action{Type: actions.Type(255)}))
	require.Zero(t, log.Len())
	require.Equal(t, uint64(1), log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, nil))
}

// Recording keeps supplied timestamps and usage values as-is.
func TestTypedPayloadValues(t *testing.T) {
	timestamp := time.Date(2026, time.October, 14, 12, 30, 0, 0, time.FixedZone("local", -4*60*60))
	var log actions.Actions
	log.Add(t.Context(), &actions.Action{
		Type: actions.TypeGenerationResponse, Timestamp: timestamp,
		GenerationResponse: &provider.Response{Created: timestamp, Usage: provider.Usage{APICost: math.Inf(1)}},
	})

	entry := log.Entries()[0]
	require.Equal(t, timestamp, entry.Timestamp)
	require.Equal(t, timestamp, entry.GenerationResponse.Created)
	require.Equal(t, math.Inf(1), entry.GenerationResponse.Usage.APICost)
}

// Verifies that nil and zero log values behave as expected.
func TestNilAndZeroLog(t *testing.T) {
	var zero actions.Actions
	require.True(t, zero.IsZero())
	require.Zero(t, zero.Len())
	require.Nil(t, zero.Messages())

	var nilLog *actions.Actions
	require.Nil(t, nilLog.Messages())
	require.Nil(t, nilLog.Entries())
	require.Zero(t, nilLog.Len())
	require.Empty(t, nilLog.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, nil, nil))
	require.Empty(t, nilLog.AddGenerationRequest(t.Context(), nil))

	var nilEvent *actions.Action
	require.Nil(t, nilEvent.Clone())
}

// Message identity depends on the action type, not whether it has prompts yet.
func TestIsMessage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action *actions.Action
		want   bool
	}{
		{
			name: "nil",
		},
		{
			name:   "zero",
			action: &actions.Action{},
		},
		{
			name:   "empty message",
			action: &actions.Action{Type: actions.TypeMessage},
			want:   true,
		},
		{
			name: "messages",
			action: &actions.Action{Type: actions.TypeMessage, Messages: prompts.Prompts{
				message(prompts.RoleUser, "first"), message(prompts.RoleUser, "second"),
			}},
			want: true,
		},
		{
			name: "debug payload",
			action: &actions.Action{Type: actions.TypeGenerationResponse, Messages: prompts.Prompts{
				message(prompts.RoleAssistant, "not delivered"),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.action.IsMessage())
		})
	}
}

// Only TypeMessage actions add prompts to the conversation. Request, response,
// and guard actions do not, even if they contain prompts. A message action still
// contributes its prompts when it also records an error.
func TestMessages(t *testing.T) {
	question := message(prompts.RoleUser, "question")
	answer := message(prompts.RoleAssistant, "answer")
	messages := prompts.Prompts{
		question, nil,
		{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Content: "reasoning"},
		message(prompts.RoleDeveloper, "developer"),
		{Role: prompts.RoleAssistant, Type: prompts.TypeUnknown, Content: "unknown"},
	}
	repeated := &actions.Action{
		Type: actions.TypeMessage, CorrelationID: "generation-1",
		Messages: prompts.Prompts{answer},
	}

	// These actions carry prompts, but their types keep them out of the conversation.
	var ignored []*actions.Action
	for _, kind := range []actions.Type{
		actions.TypeGenerationRequest, actions.TypeGenerationResponse,
		actions.TypeCapabilityRequest, actions.TypeCapabilityResponse,
		actions.TypeInputGuard, actions.TypeOutputGuard, actions.TypeError,
	} {
		ignored = append(ignored, &actions.Action{Type: kind, Messages: messages})
	}

	timestamp := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		events []*actions.Action
		want   prompts.Prompts
	}{
		{
			name:   "nil messages",
			events: []*actions.Action{{Type: actions.TypeMessage}},
		},
		{
			name:   "empty messages",
			events: []*actions.Action{{Type: actions.TypeMessage, Messages: prompts.Prompts{}}},
			want:   prompts.Prompts{},
		},
		{
			name:   "preserve roles types and nil prompts",
			events: []*actions.Action{{Type: actions.TypeMessage, Messages: messages}},
			want:   messages,
		},
		{
			name:   "ignore non-message actions",
			events: ignored,
		},
		{
			name: "message with an error",
			events: []*actions.Action{{
				Type: actions.TypeMessage, Messages: prompts.Prompts{answer}, Error: "diagnostic",
			}},
			want: prompts.Prompts{answer},
		},
		{
			name:   "same message and correlation twice",
			events: []*actions.Action{repeated, repeated},
			want:   prompts.Prompts{answer, answer},
		},
		{
			name: "append order rather than timestamp order",
			events: []*actions.Action{
				{Type: actions.TypeMessage, Timestamp: timestamp, Messages: prompts.Prompts{question}},
				{Type: actions.TypeMessage, Timestamp: timestamp.Add(-time.Hour), Messages: prompts.Prompts{answer}},
			},
			want: prompts.Prompts{question, answer},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log actions.Actions
			for _, event := range tc.events {
				log.Add(t.Context(), event)
			}

			require.Equal(t, tc.want, log.Messages())
		})
	}
}

// Execution events preserve debugging payloads but never emit transcript content.
// Explicit message actions carry reasoning, calls, and processed tool results.
func TestConversation(t *testing.T) {
	input := message(prompts.RoleUser, "question")
	answer := message(prompts.RoleAssistant, "answer")
	reasoning := &prompts.Prompt{
		Role:    prompts.RoleAssistant,
		Type:    prompts.TypeReasoning,
		Content: "reasoning",
	}

	// The model requests a tool; its result will be delivered in a separate message.
	call := prompts.ToolCall{CallID: "tool-1", Name: "lookup", Arguments: `{}`}
	calls := &prompts.Prompt{
		Role:      prompts.RoleAssistant,
		Type:      prompts.TypeToolCall,
		ToolCalls: []prompts.ToolCall{call},
	}
	result := &prompts.Prompt{
		Role: prompts.RoleTool,
		Type: prompts.TypeMessage,
		ToolResults: []prompts.ToolResponse{{
			CallID:  call.CallID,
			Content: "found",
		}},
	}

	// Record the first model turn, then explicitly deliver its reasoning and call.
	var log actions.Actions
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, prompts.Prompts{input})
	turn := log.AddGenerationRequest(t.Context(), &provider.Request{Input: prompts.Prompts{input}})
	response := &provider.Response{Output: prompts.Prompts{reasoning, calls}}
	log.AddGenerationResponse(t.Context(), turn, response, nil)
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner,
		response.Output, actions.WithCorrelation(turn))

	// Tool execution is distinct from delivering its result to the conversation.
	log.AddCapabilityRequest(t.Context(), call)
	log.AddCapabilityResponse(t.Context(), call.CallID, &result.ToolResults[0], nil)
	log.AddMessage(t.Context(), actions.EntityCapability, actions.EntityModel,
		prompts.Prompts{result}, actions.WithCorrelation(call.CallID))

	before := log.Messages()
	require.Equal(t, prompts.Prompts{input, reasoning, calls, result}, before)

	// Repeated execution payloads and cumulative request history add no messages.
	log.AddCapabilityResponse(t.Context(), call.CallID, &result.ToolResults[0], nil)
	turn = log.AddGenerationRequest(t.Context(), &provider.Request{Input: before})
	log.AddGenerationResponse(t.Context(), turn,
		&provider.Response{Output: prompts.Prompts{answer}}, nil)

	require.Equal(t, before, log.Messages())

	// The final answer appears only after its message action is recorded.
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner,
		prompts.Prompts{answer}, actions.WithCorrelation(turn))

	require.Equal(t, prompts.Prompts{input, reasoning, calls, result, answer}, log.Messages())

}

// Cloning preserves metadata types and copies the same containers as prompts do.
// Custom application objects are deliberately shared, not serialized or inspected.
func TestActionClone(t *testing.T) {
	opaque := &struct{ Value string }{Value: "read-only"}
	original := &actions.Action{
		Type: actions.TypeMessage,
		Messages: prompts.Prompts{nil, {
			Meta: map[string]any{
				"count":  7,
				"nested": map[string]any{"labels": []any{"original"}},
				"opaque": opaque,
			},
			ToolCalls:   []prompts.ToolCall{{CallID: "call", Name: "lookup"}},
			ToolResults: []prompts.ToolResponse{{CallID: "call", Content: "result"}},
		}},
	}

	cloned := original.Clone()
	require.Equal(t, original, cloned)
	require.NotSame(t, original, cloned)
	require.Same(t, opaque, cloned.Messages[1].Meta["opaque"])

	// Mutating the copied prompt's containers must not affect the source action.
	cloned.Messages[1].Meta["nested"].(map[string]any)["labels"].([]any)[0] = "changed"
	cloned.Messages[1].ToolCalls[0].Name = "changed"
	cloned.Messages[1].ToolResults[0].Content = "changed"
	require.Equal(t, "original", original.Messages[1].Meta["nested"].(map[string]any)["labels"].([]any)[0])
	require.Equal(t, "lookup", original.Messages[1].ToolCalls[0].Name)
	require.Equal(t, "result", original.Messages[1].ToolResults[0].Content)
}

// Parameter ownership matches provider.Request.Clone: the map is copied, while
// values retain their existing ownership instead of being serialized or inspected.
func TestActionCloneParameters(t *testing.T) {
	opaque := &struct{ Value string }{Value: "read-only"}
	original := &actions.Action{
		Type: actions.TypeGenerationRequest,
		GenerationRequest: &provider.Request{Params: params.New(map[string]any{
			"temperature": 0.5, "extension": opaque,
		})},
	}

	cloned := original.Clone()
	value, ok := cloned.GenerationRequest.Params.Get("extension")
	require.True(t, ok)
	require.Same(t, opaque, value)

	cloned.GenerationRequest.Params.Set("temperature", 0.75)
	value, ok = original.GenerationRequest.Params.Get("temperature")
	require.True(t, ok)
	require.Equal(t, 0.5, value)
}

// Standalone clones capture attachment metadata without clearing or retaining the
// original provider files. Repeated clones must not duplicate that metadata.
func TestActionCloneAttachments(t *testing.T) {
	file := &attachments.Attachment{Filename: "file.txt", Data: []byte("abc")}
	for _, original := range []*actions.Action{
		{Type: actions.TypeGenerationRequest, GenerationRequest: &provider.Request{Attachments: attachments.Attachments{file}}},
		{Type: actions.TypeGenerationResponse, GenerationResponse: &provider.Response{Attachments: attachments.Attachments{file}}},
	} {
		t.Run(original.Type.String(), func(t *testing.T) {
			cloned := original.Clone()
			require.Equal(t, []actions.Attachment{actions.RecordAttachment(file)}, cloned.Attachments)
			require.Empty(t, original.Attachments)
			if original.GenerationRequest != nil {
				require.Nil(t, cloned.GenerationRequest.Attachments)
				require.Same(t, file, original.GenerationRequest.Attachments[0])
			} else {
				require.Nil(t, cloned.GenerationResponse.Attachments)
				require.Same(t, file, original.GenerationResponse.Attachments[0])
			}

			require.Equal(t, cloned, cloned.Clone())
		})
	}
}

// Concurrent writers must allocate unique generation correlations and sequences.
// Readers fetch, mutate their own snapshots, and serialize while recording runs.
func TestConcurrentRecording(t *testing.T) {
	const writers, readers, turns = 6, 3, 12
	log := actions.New()
	ctx := t.Context()
	start := make(chan struct{})
	errors := make(chan error, writers+readers)
	var workers sync.WaitGroup

	// A shared start barrier exercises the first append and zero-value mutex too.
	for writer := 0; writer < writers; writer++ {
		workers.Go(func() {
			<-start
			for turn := 0; turn < turns; turn++ {
				correlation := log.AddGenerationRequest(ctx, &provider.Request{Model: "model"})
				answer := fmt.Sprintf("answer-%d-%d", writer, turn)
				messages := prompts.Prompts{message(prompts.RoleAssistant, answer)}
				sequence := log.AddGenerationResponse(ctx, correlation, &provider.Response{Output: messages}, nil)
				log.AddMessage(ctx, actions.EntityModel, actions.EntityRunner, messages, actions.WithCorrelation(correlation))
				if correlation == "" || sequence == 0 {
					errors <- fmt.Errorf("writer %d failed to record turn %d", writer, turn)
					return
				}
			}
		})
	}

	for range readers {
		workers.Go(func() {
			<-start
			for range 24 {
				messages := log.Messages()
				if len(messages) > 0 {
					messages[0].Content = "reader mutation"
				}

				entries := log.Entries()
				if len(entries) > 0 {
					entries[0].Detail = "reader mutation"
				}
				log.Len()
				log.IsZero()

				data, err := json.Marshal(log)
				if err == nil {
					var decoded actions.Actions
					err = json.Unmarshal(data, &decoded)
				}
				if err != nil {
					errors <- err
					return
				}
			}
		})
	}

	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	// Recording order, not goroutine launch order, determines event identity.
	entries := log.Entries()
	require.Len(t, entries, writers*turns*3)
	requests := make(map[string]uint64)
	responses := make(map[string]bool)
	for i, entry := range entries {
		require.Equal(t, uint64(i+1), entry.Sequence)
		require.Empty(t, entry.Detail)
		switch entry.Type {
		case actions.TypeGenerationRequest:
			require.Equal(t, fmt.Sprintf("generation-%d", len(requests)+1), entry.CorrelationID)
			requests[entry.CorrelationID] = entry.Sequence
		case actions.TypeGenerationResponse:
			require.Contains(t, requests, entry.CorrelationID)
			require.False(t, responses[entry.CorrelationID])
			require.Less(t, requests[entry.CorrelationID], entry.Sequence)
			responses[entry.CorrelationID] = true
		}
	}

	require.Len(t, responses, writers*turns)
	require.Len(t, log.Messages(), writers*turns)
	for _, answer := range log.Messages() {
		require.Contains(t, answer.Content, "answer-")
	}
}

// Recording warnings must reach the logger with the caller's span context,
// allowing the configured handler to correlate diagnostics.
func TestRecordingContext(t *testing.T) {
	// Capture the contexts supplied to the real rlog handler without depending on
	// a particular trace-field formatter or modifying the global tracing provider.
	handler := &contextHandler{}
	previous := slog.Default()
	rlog.SetDefault(rlog.New(slog.New(handler)))
	t.Cleanup(func() { rlog.SetDefault(rlog.New(previous)) })

	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), span)
	var log actions.Actions

	// Each rejected append should keep the execution context in its diagnostic.
	require.Zero(t, log.Add(ctx, &actions.Action{Type: actions.TypeUnknown}))
	require.Zero(t, log.Add(ctx, nil))
	var nilLog *actions.Actions
	require.Zero(t, nilLog.AddInputGuard(ctx, nil))

	require.Len(t, handler.contexts, 3)
	for _, received := range handler.contexts {
		require.Equal(t, span, trace.SpanContextFromContext(received))
	}

	require.Zero(t, log.Len())
}

func ExampleActions() {
	var log actions.Actions
	input := prompts.Prompts{{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "Hello"}}
	log.AddMessage(context.Background(), actions.EntityUser, actions.EntityRunner, input)
	correlation := log.AddGenerationRequest(context.Background(), &provider.Request{Model: "example-model", Input: input})
	log.AddGenerationResponse(context.Background(), correlation, &provider.Response{
		Output: prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeMessage, Content: "Hi!"}},
	}, nil)

	log.AddMessage(context.Background(), actions.EntityModel, actions.EntityRunner,
		prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeMessage, Content: "Hi!"}}, actions.WithCorrelation(correlation))

	fmt.Println(log.Len(), "events")
	for _, message := range log.Messages() {
		fmt.Println(message.Role, message.Content)
	}
	// Output:
	// 4 events
	// user Hello
	// assistant Hi!
}

// Builds a standard text message for the external-package tests.
func message(role prompts.Role, text string) *prompts.Prompt {
	return &prompts.Prompt{Role: role, Type: prompts.TypeMessage, Content: text}
}

// The handler only records contexts; test assertions happen after logging returns.
type contextHandler struct {
	contexts []context.Context
}

func (h *contextHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *contextHandler) Handle(ctx context.Context, _ slog.Record) error {
	h.contexts = append(h.contexts, ctx)
	return nil
}

func (h *contextHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *contextHandler) WithGroup(string) slog.Handler      { return h }
