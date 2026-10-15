package actions_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/actions"

	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
)

// Both JSON APIs must preserve the complete fixture, not just selected fields.
// Decoding their output again must restore the same entries and conversation.
func TestJSONRoundTrip(t *testing.T) {
	const outputFixture = `{
		"usage": {
			"invocations": 1, "tool_calls": 1,
			"input_tokens": 4, "output_tokens": 6, "total_tokens": 10,
			"api_cost": 0.01, "energy_cost": 0.002
		},
		"model": {"slug": "model", "parameters": {"temperature": 0.5}},
		"output": "answer",
		"mime_type": "text/plain",
		"capabilities": ["lookup"],
		"actions": ` + jsonRoundTripFixture + `,
		"started": "2026-10-14T12:00:00Z",
		"finished": "2026-10-14T12:00:01Z",
		"outcome": "succeeded",
		"Attachments": null
	}`

	t.Run("unmarshal/marshal", func(t *testing.T) {
		// Start from known JSON so an encoding bug cannot define our expectations.
		var original task.Output
		require.NoError(t, json.Unmarshal([]byte(outputFixture), &original))
		data, err := json.Marshal(&original)
		require.NoError(t, err)
		require.JSONEq(t, outputFixture, string(data))

		// Compare every action field and prompt after decoding the encoded output.
		var restored task.Output
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, original.Actions.Entries(), restored.Actions.Entries())

		// Compare the rest of Output separately from the log's private mutex.
		want, got := original, restored
		want.Actions, got.Actions = nil, nil
		require.Equal(t, want, got)
	})

	t.Run("decode/encode", func(t *testing.T) {
		// Read the same action fixture through the reader/writer helpers.
		original, err := actions.Decode(strings.NewReader(jsonRoundTripFixture))
		require.NoError(t, err)
		var buffer bytes.Buffer
		require.NoError(t, original.Encode(&buffer))
		require.JSONEq(t, jsonRoundTripFixture, buffer.String())

		restored, err := actions.Decode(&buffer)
		require.NoError(t, err)
		require.Equal(t, original.Entries(), restored.Entries())

		// Appending must continue both counters rather than reuse fixture IDs.
		require.Equal(t, "generation-8", restored.AddGenerationRequest(t.Context(), &provider.Request{}))
		require.Equal(t, uint64(11), restored.AddInputGuard(t.Context(), nil))
	})
}

// Decoding is transactional: a malformed replacement must not destroy an
// existing log or its conversation. Missing sequence fields are filled consistently.
func TestDecodeValidation(t *testing.T) {
	_, err := actions.Decode(strings.NewReader(`{"not":"an array"}`))
	require.Error(t, err)

	var log actions.Actions
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntitySystem, prompts.Prompts{message(prompts.RoleUser, "original")})
	original := log.Entries()

	// Invalid replacements must leave both stored events and the view intact.
	for _, data := range []string{
		`{`, `false`, `[null]`, `[{}]`,
		`[{"type":"future_event"}]`,
		`[{"type":"message","sender":"invalid"}]`,
		`[{"type":"message","sequence":2},{"type":"message","sequence":1}]`,
		`[{"type":"message","sequence":1},{"type":"message","sequence":1}]`,
	} {
		t.Run(data, func(t *testing.T) {
			require.Error(t, json.Unmarshal([]byte(data), &log))
			require.Equal(t, original, log.Entries())
		})
	}

	// Missing bookkeeping is filled, while explicit correlation numbers survive.
	require.NoError(t, json.Unmarshal([]byte(`[{"type":"message"},{"type":"generation_request","correlation_id":"generation-12"}]`), &log))
	require.Equal(t, uint64(1), log.Entries()[0].Sequence)
	require.Equal(t, uint64(2), log.Entries()[1].Sequence)
	require.Equal(t, "generation-13", log.AddGenerationRequest(t.Context(), nil))

	// Null deliberately resets the log and its counters.
	require.NoError(t, json.Unmarshal([]byte(`null`), &log))
	require.True(t, log.IsZero())
	require.Nil(t, log.Messages())
	require.Equal(t, "generation-1", log.AddGenerationRequest(t.Context(), nil))
}

// Encoding and decoding an empty log preserves all entries and messages.
func TestEmptyLogJSON(t *testing.T) {
	var log actions.Actions
	data, err := json.Marshal(&log)
	require.NoError(t, err)
	require.JSONEq(t, `[]`, string(data))

	// Both an absent log and an initialized empty log omit the envelope field.
	output := &task.Output{}
	data, err = json.Marshal(output)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"actions"`)

	output.Actions = actions.New()
	data, err = json.Marshal(output)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"actions"`)

	var decoded actions.Actions
	require.NoError(t, json.Unmarshal([]byte(`[]`), &decoded))
	require.True(t, decoded.IsZero())
}

// A bad action must not prevent task output or neighboring actions from being
// returned. Only the encoded entry changes; in-memory history stays unchanged.
func TestEncodingFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		event     *actions.Action
		wantError string
	}{
		{
			name: "unsupported metadata",
			event: &actions.Action{Type: actions.TypeMessage, Messages: prompts.Prompts{{
				Meta: map[string]any{"unsupported": make(chan int)},
			}}},
			wantError: "unsupported type: chan int",
		},
		{
			name: "unsupported parameter",
			event: &actions.Action{Type: actions.TypeGenerationRequest, GenerationRequest: &provider.Request{
				Params: params.New(map[string]any{"unsupported": make(chan int)}),
			}},
			wantError: "unsupported type: chan int",
		},
		{
			name: "non-finite usage",
			event: &actions.Action{Type: actions.TypeGenerationResponse, GenerationResponse: &provider.Response{
				Usage: provider.Usage{APICost: math.Inf(1)},
			}},
			wantError: "unsupported value: +Inf",
		},
		{
			name: "unencodable timestamp",
			event: &actions.Action{
				Type: actions.TypeInputGuard, Timestamp: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
			},
			wantError: "year outside of range",
		},
		{
			name: "custom marshaler error",
			event: &actions.Action{Type: actions.TypeMessage, Messages: prompts.Prompts{{
				Meta: map[string]any{"custom": callbackMarshaler(func() ([]byte, error) {
					return nil, errors.New("metadata encoding failed")
				})},
			}}},
			wantError: "metadata encoding failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Surround the bad entry with valid messages to check both neighbors
			// and the conversation survive serialization.
			log := actions.New()
			question := prompts.Prompts{message(prompts.RoleUser, "question")}
			answer := prompts.Prompts{message(prompts.RoleAssistant, "answer")}
			log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, question)
			tc.event.CorrelationID = "generation-7"
			log.Add(t.Context(), tc.event)
			log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner, answer)
			original := log.Entries()
			output := task.Output{Output: "answer", Outcome: task.OutcomeSucceeded, Actions: log}

			data, err := json.Marshal(&output)
			require.NoError(t, err)
			var decoded task.Output
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.Equal(t, output.Output, decoded.Output)
			require.Equal(t, output.Outcome, decoded.Outcome)

			// Replace only the failed entry, retaining its identity and relationship.
			entries := decoded.Actions.Entries()
			require.Len(t, entries, 3)
			require.Equal(t, original[0], entries[0])
			require.Equal(t, original[2], entries[2])

			failed := entries[1]
			require.Equal(t, actions.TypeError, failed.Type)
			require.Equal(t, actions.EntitySystem, failed.Sender)
			require.Equal(t, actions.EntitySystem, failed.Receiver)
			require.Equal(t, original[1].Sequence, failed.Sequence)
			require.Equal(t, original[1].CorrelationID, failed.CorrelationID)
			require.False(t, failed.Timestamp.IsZero())
			require.Equal(t, "could not encode "+tc.event.Type.String()+" action", failed.Detail)
			require.True(t, strings.HasPrefix(failed.Error, "error encoding action: "))
			require.Contains(t, failed.Error, tc.wantError)
			require.Equal(t, prompts.Prompts{question[0], answer[0]}, decoded.Messages())

			// Encoding must leave the original bad payload in memory.
			require.Equal(t, 3, log.Len())
			require.Equal(t, tc.event.Type, log.Entries()[1].Type)
			_, err = json.Marshal(log.Entries()[1])
			require.Error(t, err, "the original payload must remain unchanged")
		})
	}
}

// Multiple failures are independent, including a failed message action. Valid
// messages still form a usable transcript, with both diagnostics kept in order.
func TestMultipleEncodingFailures(t *testing.T) {
	log := actions.New()
	question := prompts.Prompts{message(prompts.RoleUser, "question")}
	answer := prompts.Prompts{message(prompts.RoleAssistant, "answer")}

	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, question)
	log.AddGenerationResponse(t.Context(), "generation-1", &provider.Response{
		Usage: provider.Usage{APICost: math.Inf(1)},
	}, nil)
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner, prompts.Prompts{{
		Role: prompts.RoleAssistant, Type: prompts.TypeMessage,
		Content: "unencodable", Meta: map[string]any{"unsupported": make(chan int)},
	}})
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner, answer)

	data, err := json.Marshal(log)
	require.NoError(t, err)
	var decoded actions.Actions
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Neither failure may replace the other or displace a valid message.
	entries := decoded.Entries()
	require.Len(t, entries, 4)
	require.Equal(t, actions.TypeError, entries[1].Type)
	require.Equal(t, uint64(2), entries[1].Sequence)
	require.Contains(t, entries[1].Error, "unsupported value: +Inf")
	require.Equal(t, actions.TypeError, entries[2].Type)
	require.Equal(t, uint64(3), entries[2].Sequence)
	require.Contains(t, entries[2].Error, "unsupported type: chan int")
	require.Equal(t, prompts.Prompts{question[0], answer[0]}, decoded.Messages())
}

// A payload error becomes an error action; a writer error is returned.
func TestEncodeErrors(t *testing.T) {
	log := actions.New()
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, prompts.Prompts{{
		Meta: map[string]any{"unsupported": make(chan int)},
	}})

	t.Run("payload error", func(t *testing.T) {
		var buffer bytes.Buffer
		require.NoError(t, log.Encode(&buffer))
		decoded, err := actions.Decode(&buffer)
		require.NoError(t, err)

		entries := decoded.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, actions.TypeError, entries[0].Type)
		require.Contains(t, entries[0].Error, "unsupported type: chan int")
	})

	t.Run("writer error", func(t *testing.T) {
		writeErr := errors.New("writer failed")
		err := log.Encode(callbackWriter(func([]byte) (int, error) { return 0, writeErr }))
		require.ErrorIs(t, err, writeErr)
	})

	// Neither encoding attempt should consume a sequence number.
	require.Equal(t, uint64(2), log.AddInputGuard(t.Context(), nil))
}

// Replacing the log while other goroutines read or encode it must be race-free.
func TestConcurrentViewsAndReplacement(t *testing.T) {
	log := actions.New()
	ctx := t.Context()
	data := []byte(`[{"sequence":1,"type":"message","messages":[{"role":"user","type":"message","content":"current"},{"role":"developer","type":"message","content":"hidden"}]}]`)
	start := make(chan struct{})
	errors := make(chan error, 3)
	var workers sync.WaitGroup

	// Replace the contents repeatedly while other goroutines read the same log.
	workers.Go(func() {
		<-start
		for range 40 {
			if err := json.Unmarshal(data, log); err != nil {
				errors <- err
				return
			}
			log.AddInputGuard(ctx, nil)
		}
	})
	workers.Go(func() {
		<-start
		for range 80 {
			log.Entries()
		}
	})
	workers.Go(func() {
		<-start
		for range 80 {
			log.Messages()
			if _, err := json.Marshal(log); err != nil {
				errors <- err
				return
			}
		}
	})

	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	// Finish with a known replacement and verify both messages were restored.
	require.NoError(t, json.Unmarshal(data, log))
	require.Equal(t, prompts.Prompts{
		message(prompts.RoleUser, "current"),
		message(prompts.RoleDeveloper, "hidden"),
	}, log.Messages())
}

// A custom marshaler or writer can append to the log during encoding.
// Neither callback should deadlock on the log's mutex.
func TestJSONCallbacksOutsideLock(t *testing.T) {
	ctx := t.Context()
	log := actions.New()
	finished := make(chan error, 1)
	marshals := 0

	go func() {
		log.AddGenerationRequest(ctx, &provider.Request{
			Params: params.New(map[string]any{
				"callback": callbackMarshaler(func() ([]byte, error) {
					marshals++
					log.AddInputGuard(ctx, nil)
					return []byte(`"captured"`), nil
				}),
			}),
		})

		// The writer appends after the JSON snapshot has already been captured.
		finished <- log.Encode(callbackWriter(func(data []byte) (int, error) {
			log.AddInputGuard(ctx, nil)
			return len(data), nil
		}))
	}()

	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("JSON callback blocked on the log lock")
	}

	// Only encoding calls the marshaler; copying or reading the log does not.
	require.Equal(t, 3, log.Len())
	require.Equal(t, actions.TypeGenerationRequest, log.Entries()[0].Type)
	require.Equal(t, 1, marshals)
}

// Arbitrary decoded JSON must re-encode safely. Each encodable action survives;
// only entries that cannot be re-encoded are replaced with diagnostics.
// JSON omission treats empty optional metadata and nil metadata equivalently.
func FuzzJSON(f *testing.F) {
	for _, seed := range []string{
		`[]`, `null`, `[null]`, `[{"type":"message"}]`,
		`[{"type":"message","messages":[]}]`,
		`[{"type":"message","messages":[{"role":"user","type":"message","meta":{}}]}]`,
		`[{"type":"generation_request","sequence":2}]`,
		`[{"type":"message","timestamp":"2026-10-14T00:00:00+24:00","messages":[{"role":"user","type":"message","content":"question"}]}]`,
		`[{"type":"message"},{"type":"message","timestamp":"2026-10-14T00:00:00+24:00"},{"type":"message"}]`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data string) {
		var log actions.Actions
		if json.Unmarshal([]byte(data), &log) != nil {
			return
		}

		encoded, err := json.Marshal(&log)
		require.NoError(t, err)
		var decoded actions.Actions
		require.NoError(t, json.Unmarshal(encoded, &decoded))

		before, after := log.Entries(), decoded.Entries()
		require.Len(t, after, len(before))

		for i, event := range before {
			// Some decoded timestamps cannot be re-encoded by time.Time. Only
			// the affected entry should become an error, retaining its identity.
			beforeJSON, payloadErr := json.Marshal(event)
			if payloadErr != nil {
				require.Equal(t, actions.TypeError, after[i].Type)
				require.Equal(t, event.Sequence, after[i].Sequence)
				require.Equal(t, event.CorrelationID, after[i].CorrelationID)
				require.Equal(t, "error encoding action: "+payloadErr.Error(), after[i].Error)
				require.Empty(t, after[i].Messages)
				continue
			}

			// Compare the complete valid entry, including its message payload.
			afterJSON, err := json.Marshal(after[i])
			require.NoError(t, err)
			require.JSONEq(t, string(beforeJSON), string(afterJSON))
		}
	})
}

// Let tests supply custom JSON behavior without defining a separate type per case.
type callbackMarshaler func() ([]byte, error)

func (f callbackMarshaler) MarshalJSON() ([]byte, error) { return f() }

type callbackWriter func([]byte) (int, error)

func (f callbackWriter) Write(data []byte) (int, error) { return f(data) }

// Fixed JSON makes field names, omitted values, payloads, and event order part of
// the round-trip test. Attachment bytes belong outside the log.
const jsonRoundTripFixture = `[
	{
		"sequence": 1,
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "message",
		"sender": "user",
		"receiver": "runner",
		"detail": "User input",
		"messages": [{
			"id": "question-1",
			"index": 0,
			"role": "user",
			"content": "question",
			"status": "completed",
			"phase": "",
			"type": "message",
			"meta": {"source": {"name": "client"}, "tags": ["question"]}
		}]
	},
	{
		"sequence": 2,
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "input_guard",
		"sender": "input_guard",
		"receiver": "runner",
		"detail": "Input accepted"
	},
	{
		"sequence": 3,
		"correlation_id": "generation-7",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "generation_request",
		"sender": "runner",
		"receiver": "model",
		"attachments": [{
			"id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			"filename": "file.txt",
			"content_type": "text/plain",
			"url": "https://example.com/file.txt",
			"size": 3,
			"sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
		}],
		"generation_request": {
			"Model": "model",
			"Params": {"temperature": 0.5, "max_output_tokens": 128},
			"Input": [{
				"id": "question-1",
				"index": 0,
				"role": "user",
				"content": "question",
				"status": "completed",
				"phase": "",
				"type": "message",
				"meta": {"source": {"name": "client"}, "tags": ["question"]}
			}],
			"Attachments": null,
			"OutputSchema": {
				"name": "answer",
				"description": "The answer format",
				"strict": true,
				"mimetype": "application/schema+json",
				"version": "1.0.0",
				"uri": "https://example.com/answer.schema.json",
				"data": "{\"type\":\"object\"}"
			},
			"Tools": [{
				"Name": "lookup",
				"Description": "Find context",
				"InputSchema": {
					"name": "lookup-input",
					"mimetype": "application/schema+json",
					"version": "1.0.0",
					"data": "{\"type\":\"object\"}"
				}
			}]
		}
	},
	{
		"sequence": 4,
		"correlation_id": "generation-7",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "generation_response",
		"sender": "model",
		"receiver": "runner",
		"generation_response": {
			"ID": "response-1",
			"Model": "model",
			"Created": "2026-10-14T12:00:00Z",
			"Usage": {"InputTokens": 4, "OutputTokens": 6, "TotalTokens": 10, "APICost": 0.01},
			"Output": [{
				"id": "answer-1",
				"index": 0,
				"role": "assistant",
				"content": "answer",
				"status": "completed",
				"phase": "final_answer",
				"type": "message",
				"citations": [{
					"StartIndex": 0,
					"EndIndex": 6,
					"Title": "Reference",
					"URL": "https://example.com/reference"
				}],
				"meta": {"confidence": 0.75}
			}],
			"Attachments": null,
			"Meta": {"request": {"id": "provider-request-1"}, "labels": ["complete"]}
		}
	},
	{
		"sequence": 5,
		"correlation_id": "generation-7",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "output_guard",
		"sender": "output_guard",
		"receiver": "runner",
		"detail": "Output accepted"
	},
	{
		"sequence": 6,
		"correlation_id": "tool-1",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "capability_request",
		"sender": "runner",
		"receiver": "capability",
		"capability_request": {"call_id": "tool-1", "name": "lookup", "arguments": "{}"}
	},
	{
		"sequence": 7,
		"correlation_id": "tool-1",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "capability_response",
		"sender": "capability",
		"receiver": "runner",
		"error": "diagnostic",
		"capability_response": {"call_id": "tool-1", "content": "result"}
	},
	{
		"sequence": 8,
		"correlation_id": "generation-7",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "message",
		"sender": "model",
		"receiver": "runner",
		"messages": [
			{
				"id": "reasoning-1",
				"index": 0,
				"role": "assistant",
				"content": "reasoning",
				"status": "completed",
				"phase": "commentary",
				"type": "reasoning",
				"meta": {"summary": ["checked context"]}
			},
			{
				"id": "call-1",
				"index": 1,
				"role": "assistant",
				"content": "",
				"status": "completed",
				"phase": "",
				"type": "tool_call",
				"tool_calls": [{"call_id": "tool-1", "name": "lookup", "arguments": "{}"}]
			},
			{
				"id": "answer-1",
				"index": 2,
				"role": "assistant",
				"content": "answer",
				"status": "completed",
				"phase": "final_answer",
				"type": "message",
				"citations": [{
					"StartIndex": 0,
					"EndIndex": 6,
					"Title": "Reference",
					"URL": "https://example.com/reference"
				}],
				"meta": {"confidence": 0.75}
			}
		]
	},
	{
		"sequence": 9,
		"correlation_id": "tool-1",
		"timestamp": "2026-10-14T12:00:00Z",
		"type": "message",
		"sender": "capability",
		"receiver": "model",
		"messages": [{
			"id": "result-1",
			"index": 0,
			"role": "tool",
			"content": "",
			"status": "completed",
			"phase": "",
			"type": "message",
			"meta": {"source": "lookup"},
			"tool_results": [{"call_id": "tool-1", "content": "result"}]
		}]
	}
]`
