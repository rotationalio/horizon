package prompts

// Identifies the kind of content represented by a prompt.
//
//go:generate enumify -names typeNames
type Type uint8

// Provider-neutral prompt content types used by Horizon.
const (
	TypeUnknown Type = iota
	TypeMessage
	TypeToolCall
	TypeRefusal
	TypeContentFilter
	TypeReasoning
)

var typeNames = []string{
	"unknown",
	"message",
	"tool_call",
	"refusal",
	"content_filter",
	"reasoning",
}

// IsKnown reports whether t is a supported, non-unknown prompt type.
func (t Type) IsKnown() bool {
	return t > TypeUnknown && t < Type(len(typeNames))
}
