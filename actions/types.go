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
}
