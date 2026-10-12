package prompts

import "slices"

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

// Clone returns an independent copy of the prompt's mutable fields. JSON-like
// values in Meta are recursively copied; opaque metadata values remain shared.
func (p *Prompt) Clone() *Prompt {
	if p == nil {
		return nil
	}

	cloned := *p
	cloned.Citations = slices.Clone(p.Citations)
	cloned.ToolCalls = slices.Clone(p.ToolCalls)
	cloned.ToolResults = slices.Clone(p.ToolResults)

	// Recursively clone the Meta map, preserving nil entries and order.
	// Stdlib maps.Clone will only clone the map's keys and values, not the
	// values themselves.
	if p.Meta != nil {
		cloned.Meta = make(map[string]any, len(p.Meta))
		for key, value := range p.Meta {
			cloned.Meta[key] = cloneMetaValue(value)
		}
	}
	return &cloned
}

// Clone returns an independent copy of each prompt, preserving nil entries and order.
func (p Prompts) Clone() Prompts {
	if p == nil {
		return nil
	}

	cloned := make(Prompts, len(p))
	for i, prompt := range p {
		cloned[i] = prompt.Clone()
	}
	return cloned
}

// cloneMetaValue recursively copies JSON-like metadata containers. Values
// that are not map or slice types are returned as-is.
func cloneMetaValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, nested := range value {
			cloned[key] = cloneMetaValue(nested)
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, nested := range value {
			cloned[i] = cloneMetaValue(nested)
		}
		return cloned
	case []string:
		return slices.Clone(value)
	case []byte:
		return slices.Clone(value)
	default:
		return value
	}
}

type Citation struct {
	StartIndex int64
	EndIndex   int64
	Title      string
	URL        string
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

// Returns true if the prompts are sorted by index, otherwise returns false.
func (p Prompts) IsSorted() bool {
	for i := range p {
		if i > 0 && p[i].Index < p[i-1].Index {
			return false
		}
	}
	return true
}

// FilterOutTypes returns a new slice with the given [Type]s removed, preserving
// order and nil entries. The retained prompts themselves are not copied.
func (p Prompts) FilterOutTypes(types ...Type) Prompts {
	if p == nil {
		return nil
	}
	out := make(Prompts, 0, len(p))
	for _, prompt := range p {
		if prompt == nil || !slices.Contains(types, prompt.Type) {
			out = append(out, prompt)
		}
	}
	return out
}

// FilterOutRoles returns a new slice with the given [Role]s removed, preserving
// order and nil entries. The retained prompts themselves are not copied.
func (p Prompts) FilterOutRoles(roles ...Role) Prompts {
	if p == nil {
		return nil
	}
	out := make(Prompts, 0, len(p))
	for _, prompt := range p {
		if prompt == nil || !slices.Contains(roles, prompt.Role) {
			out = append(out, prompt)
		}
	}
	return out
}
