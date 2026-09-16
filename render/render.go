package render

import (
	"fmt"
)

// Renderer is the primary interface for converting prompt templates into a fully
// rendered input for the LLM. They are only responsible for producing text context.
// Renderers should be stateful and reusable because often templates must be parsed
// before they can be rendered. Stateful renderers allow reuse of the same parsed
// template with LRU caches to avoid repeated parsing of the same template.
type Renderer interface {
	// Render the template with the provided context data.
	Render(map[string]any) (string, error)
}

// Creates a new [Renderer] from a string template with the provided renderer
// [Type] or a string representation of the renderer [Type], returning an error
// if the renderer is not supported.
func New(renderer any, template string) (_ Renderer, err error) {
	var t Type
	if t, err = ParseType(renderer); err != nil {
		return nil, err
	}

	switch t {
	case Sprintf:
		return NewFormat(template)
	case Go:
		return NewTemplate(template)
	default:
		return nil, fmt.Errorf("unsupported renderer type: %s", t.String())
	}
}

// an enum of the template format type, the string representation of the template format
// type, or a Renderer interface that will be used to render the template and data,
// validating the template and data if it also implements the Validator interface.
func Render(renderer any, template string, data map[string]any) (_ string, err error) {
	var rendering Renderer
	switch val := renderer.(type) {
	case Renderer:
		rendering = val
	case Type:
		switch val {
		case Sprintf:
			if rendering, err = NewFormat(template); err != nil {
				return "", err
			}
		case Jinja:
			return "", fmt.Errorf("jinja templates are not currently supported")
		case Go:
			if rendering, err = NewTemplate(template); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unsupported template format: %s", val)
		}
	case string:
		// Parse the template format type from the string.
		var kind Type
		if kind, err = ParseType(val); err != nil {
			return "", err
		}
		return Render(kind, template, data)
	default:
		return "", fmt.Errorf("unsupported renderer type: %T", renderer)
	}

	// Render the template using the renderer.
	return rendering.Render(data)
}

// Check if a template is valid for a given renderer type.
// This is basically a shortcut for _, err := New(renderer, template); err == nil.
func Validate(renderer any, template string) (err error) {
	_, err = New(renderer, template)
	return err
}
