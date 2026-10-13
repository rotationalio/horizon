package templates

import (
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/render"
)

// A collection of [Template]s.
type Templates []*Template

// A prompt template that can be rendered into a [prompts.Prompt] via a
// [prompts.Context] and a [render.Renderer].
type Template struct {
	Index    uint         `json:"index" yaml:"index" msg:"index"`          // The index of the template in the list of templates
	Role     prompts.Role `json:"role" yaml:"role" msg:"role"`             // The role of the template
	Content  string       `json:"content" yaml:"content" msg:"content"`    // The content of the template
	Renderer render.Type  `json:"renderer" yaml:"renderer" msg:"renderer"` // The renderer type to use for the template
}

// IsSorted reports whether the templates are sorted by index.
func (t Templates) IsSorted() bool {
	for i := range t {
		if i > 0 && t[i].Index < t[i-1].Index {
			return false
		}
	}
	return true
}

// Render applies the default cached renderer to the template collection.
func (t Templates) Render(ctx prompts.Context) (prompts.Prompts, error) {
	return Render(t, ctx)
}
