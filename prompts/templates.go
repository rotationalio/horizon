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

// Returns true if the templates are sorted by index, otherwise returns false.
func (t Templates) IsSorted() bool {
	for i := range t {
		if i > 0 && t[i].Index < t[i-1].Index {
			return false
		}
	}
	return true
}
