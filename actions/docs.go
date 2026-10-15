// Package actions records what happens during a Horizon execution, so you can
// inspect a run later or read its conversation while it is still in progress.
//
// There are two views of the same log:
//   - [Actions.Entries] shows everything recorded: requests, responses, guard
//     decisions, and messages.
//   - [Actions.Messages] shows only the prompts recorded with
//     [Actions.AddMessage], in order.
//
// Recording a response does not add it to the conversation. Record a message
// separately when its content is accepted and enters the conversation.
//
// # Recording a model turn
//
// Start with a log and the user's messages. Always pass the execution context
// so any logging diagnostics stay connected to the task's trace.
//
//	log := actions.New()
//	input := prompts.Prompts{{
//		Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "Hello",
//	}}
//	log.AddMessage(ctx, actions.EntityUser, actions.EntityRunner, input)
//	log.AddInputGuard(ctx, nil) // nil means accepted; an error means rejected.
//
// Record the request and response as separate events. The request helper
// returns an ID that links this turn's events together.
//
//	request := &provider.Request{Model: "model", Input: input}
//	turn := log.AddGenerationRequest(ctx, request)
//	response := &provider.Response{
//		Output: prompts.Prompts{{
//			Role: prompts.RoleAssistant, Type: prompts.TypeMessage,
//			Content: "Hi!",
//		}},
//	}
//	log.AddGenerationResponse(ctx, turn, response, nil) // No generation error.
//
// When the output has no guards to pass, add its messages to the conversation:
//
//	log.AddMessage(ctx, actions.EntityModel, actions.EntityRunner,
//		response.Output, actions.WithCorrelation(turn))
//
// # Checking output with a guard
//
// If a guard runs, use this instead of the final [Actions.AddMessage] call
// above. The raw response is already recorded; now record the decision and, if
// accepted, the messages. Rejected output stays in the generation response
// entry in [Actions.Entries], but not in the [Actions.Messages] transcript.
//
// Guards only accept or reject. Pass a clone so guard edits do not
// change the response that will be delivered.
//
//	guardErr := guard.ProtectOutput(response.Clone())
//	log.AddOutputGuard(ctx, turn, guardErr)
//	if guardErr == nil {
//		log.AddMessage(ctx, actions.EntityModel, actions.EntityRunner,
//			response.Output, actions.WithCorrelation(turn))
//	}
//
// # Recording a tool exchange
//
// Capability actions record attempted dispatches and their outcomes; they do
// not run the tool or add messages.
//
//	call := prompts.ToolCall{CallID: "tool-1", Name: "lookup", Arguments: `{}`}
//	log.AddCapabilityRequest(ctx, call)
//	result := &prompts.ToolResponse{CallID: call.CallID, Content: "Found it"}
//	log.AddCapabilityResponse(ctx, call.CallID, result, nil) // nil error
//
// When the processed result enters the conversation, record those prompts too.
// This preserves their formatting, metadata, and any extra content and will
// expose the tool calls and their results via the [Actions.Messages]
// transcript.
//
//	toolMessages := prompts.Prompts{{
//		Role: prompts.RoleTool, Type: prompts.TypeMessage,
//		ToolResults: []prompts.ToolResponse{*result},
//	}}
//	log.AddMessage(ctx, actions.EntityCapability, actions.EntityModel,
//		toolMessages, actions.WithCorrelation(call.CallID))
//
// If execution fails without sending any feedback, record only the failure:
//
//	log.AddCapabilityResponse(ctx, call.CallID, nil, executionErr)
//
// # Linking related events
//
// Use [WithCorrelation] to connect messages to their generation ID or
// tool-call ID. Each event also has its own [Action.Sequence] number, which
// identifies its place in the log.
//
// A request or response may repeat content found in a nearby message action.
// That duplication is intentional: one records execution details, the other
// records what was said. Sharing a correlation ID links them; it does not
// remove messages. If you record the same message twice, the conversation will
// show it twice.
//
// # Extra details and attachments
//
// Use [WithDetail] for a short explanation and [WithAttachments] to capture
// file metadata and a SHA-256 hash. Request and response helpers capture their
// attached files automatically. The log never keeps file bytes or pointers to
// source files, you must correlate attachment IDs to their data.
//
//	log.AddMessage(ctx, actions.EntityUser, actions.EntityRunner, input,
//		actions.WithDetail("Uploaded document"), actions.WithAttachments(file))
//	metadata := actions.RecordAttachment(file) // Capture metadata on its own.
//
// For an event not covered by a helper, construct an [Action] and use
// [Actions.Add]:
//
//	log.Add(ctx, &actions.Action{
//		Type: actions.TypeInputGuard, Sender: actions.EntityInputGuard,
//		Receiver: actions.EntityRunner, Detail: "Validated input",
//	})
//
// # Reading the log
//
// [Actions.Messages] returns [prompts.Prompts], including any reasoning or
// tool content you recorded as messages. Use the returned prompts' filters to
// hide selected types or roles without changing the log. The task output's
// Messages method returns the same conversation.
//
//	transcript := log.Messages().FilterOutTypes(prompts.TypeRefusal)
//	events := log.Entries()
//	if events[0].IsMessage() {
//		messages := events[0].Messages // All prompts in this message action.
//	}
//	count := log.Len()
//
// # Saving and loading JSON
//
// A log encodes as an ordered JSON array. Decoding replaces its contents and
// restores the counters, so you can continue recording afterward. Custom
// correlations do not advance the counter while recording; use the generation
// request helper to allocate generation IDs.
//
// Recording does not validate or normalize payload values. If an action cannot
// be encoded, only that entry becomes a system-to-system [TypeError] action;
// the rest of the log is preserved. The diagnostic keeps the original sequence
// and correlation ID, names the original action type in Detail, and records the
// encoding error. Its timestamp is when the error was detected.
//
// This keeps a bad entry from breaking the enclosing output. The in-memory log
// is unchanged, but the failed entry's payload is not saved or included in the
// decoded transcript. [Actions.Encode] still returns any writer error.
//
//	data, err := json.Marshal(log)
//	err = json.Unmarshal(data, log)
//
// Use [Actions.Encode] and [Decode] with writers and readers:
//
//	err = log.Encode(writer)
//	restored, err := actions.Decode(reader)
//
// # Sharing a log safely
//
// [New] returns a ready-to-use pointer. A local zero-value [Actions] works
// too, but pass its address to json.Marshal. Do not copy a log after using it:
// it contains a mutex. Recording on a nil log warns and skips the event rather
// than allocating.
//
// Recording and reads can run concurrently. Keep input payloads unchanged
// until the recording helper returns. Prompts, provider structs, and JSON-like
// metadata containers are copied. Parameter maps are copied too, but their
// values remain shared, as do custom objects in metadata. Treat those shared
// values as read-only. See [Action.Clone] for the copying rules.
//
// Attachments are read only while capturing their metadata and hashes. Once
// recording returns, source files and bytes can be changed freely.
//
// [Actions.Entries] and [Actions.Messages] copy one point-in-time view using
// the same rules. Concurrent appends may appear only on the next read.
//
// # Benchmarking
//
// BenchmarkProcessActions shows a comparison of processing time between a
// no-op actions log (Output.Actions artifically set to nil) and a real log,
// showing the additional requirements for cloning objects into the actions
// log. The Actions log does not appreciably add to processing time, or memory,
// especially when actual inference calls are being performed.
//
//	go test -run '^$' -bench '^BenchmarkProcessActions$' -benchmem -count=5 .
package actions
