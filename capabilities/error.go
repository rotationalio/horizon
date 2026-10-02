package capabilities

import "go.rtnl.ai/horizon/errors"

// Error sentinels
var (
	ErrInvalidName         = errors.New("invalid capability name")
	ErrInvalidToolPrompt   = errors.New("invalid tool response prompt")
	ErrNotFound            = errors.New("capability not found")
	ErrUnsupportedResponse = errors.New("unsupported capability response")
)

// Error represents a capability execution error that might be retried by the
// model.
type Error struct {
	Retryable bool   // If true, the model should be able to fix the request, otherwise it's terminal.
	Message   string // Model-safe message if Retryable is true.
	Cause     error  // Original cause of the error; do not surface to the model.
}

// Error returns the safe message without exposing the underlying cause.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Unwrap returns the original diagnostic cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
