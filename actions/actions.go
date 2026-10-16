package actions

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/x/rlog"
)

// TODO: record webhook callbacks/retries, async queue and worker handoffs, and
// task-to-task messages when those execution paths are implemented.

//=============================================================================
// Actions
//=============================================================================

// Actions is a concurrent interaction log. Its zero value is ready to use.
// See the package documentation for ownership and concurrency rules.
type Actions struct {
	mu         sync.RWMutex // Protects log bookkeeping.
	entries    []*Action    // Immutable recorded actions.
	sequence   uint64       // Last-used sequence number.
	generation uint64       // Last-used generation number.
}

// ZeroActions is the zero value for an Actions log.
var ZeroActions = &Actions{}

// New creates an empty log. A standalone zero-value Actions works equally well.
func New() *Actions { return &Actions{} }

// Add clones an event and assigns its sequence and timestamp. It returns the
// assigned sequence, or zero if the event cannot be recorded. A nil log, nil
// event, or invalid action type is logged with ctx and skipped.
func (a *Actions) Add(ctx context.Context, event *Action) uint64 {
	sequence, _ := a.appendEvent(ctx, event, false)
	return sequence
}

// Len returns the number of recorded events.
func (a *Actions) Len() int {
	if a == nil {
		return 0
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.entries)
}

// IsZero reports whether the log is empty, including a nil log.
func (a *Actions) IsZero() bool { return a.Len() == 0 }

// Entries returns event copies using [Action.Clone]'s ownership rules.
// Concurrent appends may happen after the event list is captured.
func (a *Actions) Entries() []*Action {
	if a == nil {
		return nil
	}

	entries := a.entrySnapshot()
	for i, event := range entries {
		entries[i] = event.Clone()
	}

	return entries
}

// Messages returns conversation prompts in recording order, using
// [prompts.Prompts.Clone]'s copy rules. Only [TypeMessage] events contribute;
// request/response and guard payloads are debug information, not transcript
// content. Explicitly repeated messages remain.
func (a *Actions) Messages() prompts.Prompts {
	if a == nil {
		return nil
	}

	// Capture event pointers under the lock, then copy their immutable payloads.
	entries := a.entrySnapshot()

	var messages prompts.Prompts
	for _, event := range entries {
		if event.IsMessage() && event.Messages != nil {
			// Preserve an explicitly empty conversation as an empty slice.
			if messages == nil {
				messages = make(prompts.Prompts, 0, len(event.Messages))
			}
			messages = append(messages, event.Messages...)
		}
	}

	// Returned prompts can be filtered or edited without changing the log.
	return messages.Clone()
}

// appendEvent assigns sequence and optional generation correlation together.
func (a *Actions) appendEvent(ctx context.Context, event *Action, generation bool) (uint64, string) {
	if a == nil {
		rlog.WarnContext(ctx, "cannot record an action: nil log")
		return 0, ""
	}

	if event == nil {
		rlog.WarnContext(ctx, "cannot record a nil action")
		return 0, ""
	}

	if event.Type <= TypeUnknown || int(event.Type) >= len(typeNames) {
		rlog.WarnAttrs(ctx, "cannot record an unknown action type", slog.Int("type", int(event.Type)))
		return 0, ""
	}

	// Copy payload containers before publishing the event to concurrent readers.
	owned := event.Clone()
	if owned.Timestamp.IsZero() {
		owned.Timestamp = time.Now().UTC()
	}

	// Serialize only allocation and publication, not provider/tool execution.
	a.mu.Lock()

	if generation {
		a.generation++
		owned.CorrelationID = fmt.Sprintf("generation-%d", a.generation)
	}

	a.sequence++
	owned.Sequence = a.sequence
	a.entries = append(a.entries, owned)

	a.mu.Unlock()
	return owned.Sequence, owned.CorrelationID
}

// entrySnapshot returns a clone of the log entries under a read lock.
func (a *Actions) entrySnapshot() []*Action {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return slices.Clone(a.entries)
}

//=============================================================================
// Action
//=============================================================================

// Action records one interaction or guard decision. Requests and responses are
// separate events. Only populate the payload appropriate to Type.
type Action struct {
	// * Event identity and participants, shared by all action types.

	Sequence      uint64    `json:"sequence"`                 // Event identity and append order within this execution.
	CorrelationID string    `json:"correlation_id,omitempty"` // Links related requests, responses, and decisions.
	Timestamp     time.Time `json:"timestamp"`                // When the event was recorded.
	Type          Type      `json:"type"`                     // The interaction or decision being recorded.
	Sender        Entity    `json:"sender"`                   // Participant that produced this event.
	Receiver      Entity    `json:"receiver"`                 // Participant receiving the event.

	// * Optional metadata, usable with any action type.

	Detail      string       `json:"detail,omitempty"`      // Human-readable context or decision explanation.
	Error       string       `json:"error,omitempty"`       // Normally generation/capability failures or guard rejections.
	Attachments []Attachment `json:"attachments,omitempty"` // Captured file metadata and hashes, never file data or references.

	// * TypeMessage conversation content; other event payloads do not enter the transcript.

	Messages prompts.Prompts `json:"messages,omitempty"` // TypeMessage: delivered conversation prompts, including reasoning and tools.

	// * Generation payloads; attachments are captured in the common metadata above.

	GenerationRequest  *provider.Request  `json:"generation_request,omitempty"`  // TypeGenerationRequest: model inputs, settings, and tools.
	GenerationResponse *provider.Response `json:"generation_response,omitempty"` // TypeGenerationResponse: raw output, usage, and provider metadata.

	// * Capability execution payloads, independent of delivered message actions.

	CapabilityRequest  *prompts.ToolCall     `json:"capability_request,omitempty"`  // TypeCapabilityRequest: name, call ID, and original arguments.
	CapabilityResponse *prompts.ToolResponse `json:"capability_response,omitempty"` // TypeCapabilityResponse: model-visible result for the call.
}

// ZeroAction is the zero value for an Action.
var ZeroAction = &Action{}

// Clone copies an action using the prompt and provider Clone methods. JSON-like
// metadata is copied; parameter values and opaque metadata objects remain shared
// and must be treated as read-only. File attachments become metadata and hashes,
// without copying or retaining file bytes. A nil receiver returns nil.
func (a *Action) Clone() *Action {
	if a == nil {
		return nil
	}

	cloned := *a
	cloned.Messages = a.Messages.Clone()
	cloned.Attachments = slices.Clone(a.Attachments)

	// Separate the provider structs before removing their file references. This
	// avoids copying attachment bytes that the action log does not retain.
	if a.GenerationRequest != nil {
		request := *a.GenerationRequest
		cloned.GenerationRequest = &request
	}
	if a.GenerationResponse != nil {
		response := *a.GenerationResponse
		cloned.GenerationResponse = &response
	}
	captureAttachments(&cloned)

	cloned.GenerationRequest = cloned.GenerationRequest.Clone()
	cloned.GenerationResponse = cloned.GenerationResponse.Clone()

	if a.CapabilityRequest != nil {
		call := *a.CapabilityRequest
		cloned.CapabilityRequest = &call
	}
	if a.CapabilityResponse != nil {
		result := *a.CapabilityResponse
		cloned.CapabilityResponse = &result
	}

	return &cloned
}

// IsMessage reports whether this action records conversation messages.
func (a *Action) IsMessage() bool { return a != nil && a.Type == TypeMessage }
