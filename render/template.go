package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	text "text/template"
)

// Go template engine [Renderer].
type Template struct {
	template *text.Template
}

// Ensure [Template] implements the [Renderer] interface.
var _ Renderer = &Template{}

// Creates a new [Template] from a template string.
func NewTemplate(template string) (_ *Template, err error) {
	t := &Template{}
	if t.template, err = text.New("").Funcs(funcMap).Parse(template); err != nil {
		return nil, err
	}
	return t, nil
}

// Renders a template with the given data using the [text.Template] renderer.
func (t *Template) Render(data map[string]any) (_ string, err error) {
	var buf bytes.Buffer
	if err = t.template.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

//============================================================================
// Templating Functions
//============================================================================

var funcMap = text.FuncMap{
	"jsonify": jsonify,
}

func jsonify(v any) (string, error) {
	sb := strings.Builder{}
	sb.WriteString("```json\n")

	encoder := json.NewEncoder(&sb)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return "", fmt.Errorf("failed to encode JSON: %w", err)
	}

	sb.WriteString("\n```\n")
	return sb.String(), nil
}
