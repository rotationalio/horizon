package prompts

import "go.rtnl.ai/horizon/render"

// A collection of [Template]s.
type Templates []*Template

// A prompt template that can be rendered into a [Prompt] via a [Context] and a [Renderer].
type Template struct {
	Index    uint        `json:"index" yaml:"index" msg:"index"`          // The index of the template in the list of templates
	Role     Role        `json:"role" yaml:"role" msg:"role"`             // The role of the template
	Content  string      `json:"content" yaml:"content" msg:"content"`    // The content of the template
	Renderer render.Type `json:"renderer" yaml:"renderer" msg:"renderer"` // The renderer type to use for the template
}

// A collection of [Prompt]s.
type Prompts []*Prompt

// A prompt is generally rendered from a [Template] and is a single message sent to the
// LLM as part of a request to cause it to perform a generation.
type Prompt struct {
	ID          string         `json:"id,omitempty"`
	Index       uint           `json:"index"`   // The index of the prompt in the list of prompts
	Role        Role           `json:"role"`    // The role of the message sender.
	Content     string         `json:"content"` // The rendered content of the message.
	Status      string         `json:"status"`  // Any of "in_progress", "completed", "incomplete".
	Phase       string         `json:"phase"`   // Any of "commentary", "final_answer".
	Type        Type           `json:"type"`    // The normalized prompt content type.
	Citations   []Citation     `json:"citations,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	ToolCalls   []ToolCall     `json:"tool_calls,omitempty"`   // Assistant tool calls associated with the message.
	ToolResults []ToolResponse `json:"tool_results,omitempty"` // Results associated with prior tool calls.
}

// ToolCall is a provider-neutral function call in the inference transcript.
type ToolCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResponse is a provider-neutral result correlated to an assistant tool call.
type ToolResponse struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	// TODO: support more than just textual string content here, such as files and binary data.
}

type Citation struct {
	StartIndex int64
	EndIndex   int64
	Title      string
	URL        string
}

// Identifies the kind of content represented by a prompt.
type Type string

// Standard prompt content types used by providers when normalizing responses.
const (
	TypeUnknown       Type = "unknown"
	TypeMessage       Type = "message"
	TypeToolCall      Type = "tool_call"
	TypeRefusal       Type = "refusal"
	TypeContentFilter Type = "content_filter"
)

// Returns true if the templates are sorted by index, otherwise returns false.
func (t Templates) IsSorted() bool {
	for i := range t {
		if i > 0 && t[i].Index < t[i-1].Index {
			return false
		}
	}
	return true
}

// Returns true if the prompts are sorted by index, otherwise returns false.
func (p Prompts) IsSorted() bool {
	for i := range p {
		if i > 0 && p[i].Index < p[i-1].Index {
			return false
		}
	}
	return true
}
