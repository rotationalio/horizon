package task

import (
	"time"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/x/mime"
)

// Outcome is the terminal result of a task execution.
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

type Output struct {
	Usage        Usage               `json:"usage"`
	Model        Model               `json:"model"`
	Output       any                 `json:"output,omitempty"` // Final or partial model output; Run may return it with an error.
	MimeType     mime.Type           `json:"mime_type"`
	Capabilities []capabilities.Name `json:"capabilities"`
	Actions      Actions             `json:"actions,omitempty"` // Log of important actions.
	Started      time.Time           `json:"started"`
	Finished     time.Time           `json:"finished"`
	Outcome      Outcome             `json:"outcome"`
	Attachments  attachments.Attachments
}

// TODO: Output.Messages() should sub to Actions.Messages() for user sugar

// Usage is the model and capability usage information for a task execution.
type Usage struct {
	Invocations  uint64  `json:"invocations"`   // Number of inference calls in the task execution.
	ToolCalls    uint64  `json:"tool_calls"`    // Number of capability tool calls in the task execution.
	InputTokens  int64   `json:"input_tokens"`  // Total input tokens reported by inference.
	OutputTokens int64   `json:"output_tokens"` // Total output tokens reported by inference.
	TotalTokens  int64   `json:"total_tokens"`  // Total tokens reported by inference.
	APICost      float64 `json:"api_cost"`      // Price in US dollars for the API call.
	EnergyCost   float64 `json:"energy_cost"`   // Energy cost in US dollars for the API call.
}

// Returns the duration of the task execution.
func (o *Output) Latency() time.Duration {
	if o.Started.IsZero() || o.Finished.IsZero() {
		return 0
	}
	return o.Finished.Sub(o.Started)
}

//============================================================================
// Actions Log Definition
//============================================================================
// TODO: move this to action.go?

// TODO: actions types in an enumify enum (another file)
// TODO: sender/reciever types in an enumify enum (another file)

// A collection of [Action]s taken during a task execution.
type Actions []*Action

// TODO: add Actions.Messages() that returns the messages as they were seen by the model as a replay (make sure it is returned in order)

//TODO: consider what actions we want to log and what info we want to keep
// TODO: actions are everything; guardrails might not attach a message or might edit them.
//  ! not for stuff like rendering, context processing, etc.
//  ! Use for stuff like tool calls, tool response, guardrails, webhooks, etc. (entity to entity) (aka: anything we send around and want to know what happened with)
//
// TODO: sender/reciever type (what to call this? "Entity"?)
//  * System (should we have some way to designate what part of the system?)
//  * User
//  * Model
//  * Capability (a tool)
//  * Webhook (should we have some way to designate which webhook or "whose" it was?)
//  * ???
//
// TODO: Types (or another word here instead of "types"?)
//  * Message(s) sent (has sender and reciever and must save messages or the content at least so it can be reproduced)
//  * Tool call and results
//  * Input or output guard actions
//  * Webhook calls (such as at the end of a task for async, when that is implemented)
//  * Generation request and response (is this just a Message type or it's own type?)
//  * ???

// An action is a single action taken during a task execution, such as a message
// prompt or a capability (tool/resource) call.
type Action struct {
	// TODO: actions names enum?
	Timestamp time.Time `json:"timestamp"` // The timestamp of the action
	// TODO: replace sender/reciever types with an enumify enum ("Entity"?)
	Sender   string `json:"sender"`   // The sender of the action
	Reciever string `json:"reciever"` // The recipient of the action
	// TODO: replace these content fields as needed (or keep) for reproducing as a message or other type of Prompt via Action.Message()
	// Message   *prompts.Prompt `json:"message,omitempty"` // The message prompt associated with the action
	// Params    http.JSON       `json:"params"`            // The parameters sent to the target
	// Output    http.JSON       `json:"output"`            // The output response of the action
	// Usage     *Usage          `json:"usage,omitempty"`   // Usage information for the action
}

// TODO: Action.Message() that returns the message prompt associated with the action (or nil if that action does not have a message prompt; maybe add an "ok" bool output?)
