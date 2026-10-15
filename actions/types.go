package actions

//go:generate enumify -names typeNames

// Type identifies the event recorded by an action. Requests and responses are
// separate events, including when an operation fails.
type Type uint8

const (
	TypeUnknown Type = iota
	TypeMessage
	TypeGenerationRequest
	TypeGenerationResponse
	TypeCapabilityRequest
	TypeCapabilityResponse
	TypeInputGuard
	TypeOutputGuard

	// TypeError records a diagnostic about the action log itself.
	TypeError

	// TODO: record webhook callbacks/retries, async queue and worker handoffs, and
	// task-to-task messages when those execution paths are implemented.
)

var typeNames = []string{
	"unknown",
	"message",
	"generation_request",
	"generation_response",
	"capability_request",
	"capability_response",
	"input_guard",
	"output_guard",
	"error",
}
