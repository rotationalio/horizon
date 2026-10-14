// Package actions defines the interaction log and conversation view of a Horizon
// execution. It is intended for debugging and analysis, not reproducing a run.
package actions

import (
	"time"

	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
)

// Actions is an ordered collection of events taken during a task execution.
type Actions []*Action

// Action records a single interaction or guard decision. Requests and responses
// are recorded separately, with only the payload appropriate to Type populated.
// Rendering and context processing are not actions.
type Action struct {
	ID            string    `json:"id"`                       // Identifies this event within the execution.
	Sequence      uint64    `json:"sequence"`                 // Recording order; timestamps alone do not establish order.
	CorrelationID string    `json:"correlation_id,omitempty"` // Shared by a request and its response; tool calls use their call ID.
	Timestamp     time.Time `json:"timestamp"`
	Type          Type      `json:"type"`
	Sender        Entity    `json:"sender"`
	Receiver      Entity    `json:"receiver"`
	SenderID      string    `json:"sender_id,omitempty"` // Optional component, model, user, or capability identifier.
	ReceiverID    string    `json:"receiver_id,omitempty"`

	// Messages carries standalone messages or the messages accepted/changed by a
	// guard. It retains roles, reasoning, tool calls/results, and other prompt content.
	// Guard decisions need not carry messages; Error records rejection, and Detail
	// can describe an allowed or transformed output. Input guards only validate.
	Messages prompts.Prompts `json:"messages,omitempty"`
	Detail   string          `json:"detail,omitempty"`
	Error    string          `json:"error,omitempty"` // Serializable failure or rejection reason; may accompany partial results.

	// Generation payloads retain model/settings, messages, attachments, and response
	// usage/metadata using the existing provider-neutral contracts.
	GenerationRequest  *provider.Request  `json:"generation_request,omitempty"`
	GenerationResponse *provider.Response `json:"generation_response,omitempty"`

	// Capability payloads preserve the original model arguments and the result
	// contributed to the conversation, including model-visible failure feedback.
	CapabilityRequest  *prompts.ToolCall     `json:"capability_request,omitempty"`
	CapabilityResponse *prompts.ToolResponse `json:"capability_response,omitempty"`
}

// TODO: add constructors for each action type, including messages, generation
// requests/responses, capability requests/responses, and input/output guard decisions.
// TODO: add one-line Actions.Add and event-specific recording helpers that assign
// IDs, sequence numbers, and timestamps and correlate requests with responses.
// TODO: snapshot mutable payloads when recording, before output guards can change
// the provider response; retain accepted guard output without duplicating the transcript.
// TODO: validate that each action type has the appropriate payload and participants.
// TODO: Action.Message() should return the associated prompt (or nil/ok=false);
// consider a plural helper because generation events may contain multiple messages.
// TODO: record actions for:
//   - Webhook callback requests, responses, and delivery retries.
//   - Async task enqueueing, worker handoffs, and completion notifications.
//   - Task-to-task message handoffs.

// Messages returns the conversation view in order, including provider-returned
// reasoning, tool calls, and tool results. It is not an exact provider request replay.
func (a Actions) Messages() prompts.Prompts {
	// TODO: extract messages from event payloads in order, without repeating prior
	// history from subsequent generation requests or tool messages logged twice.
	// TODO: preserve existing transcript behavior and support configured type/role
	// filters without mutating the action log or provider response slices.
	return nil
}
