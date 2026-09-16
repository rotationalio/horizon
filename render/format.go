package render

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// Find all % signs not preceded by another % sign.
// TODO: this regular expression is not working as expected, see test cases for failrues.
var directives = regexp.MustCompile(`%([^%]|$)`)

// The Format renderer (also referred to as f-string) is a simple string template
// formatter that uses C-style string formatting directives to render the template.
//
// Currently: this renderer converts the context data into an array of strings ordered
// by the keys of the context data. This array is then passed to the sprintf template.
//
// TODO: Implement python format style rendering with the use of key names.
// See: https://github.com/slongfield/pyfmt
type Format struct {
	template   string
	directives int
}

// Ensure [Format] implements the [Renderer] interface.
var _ Renderer = &Format{}

var ErrNoDirectives = errors.New("template contains no string format directives")

// Creates a new [Format] renderer from a template string.
func NewFormat(template string) (_ *Format, err error) {
	f := &Format{template: template}
	f.directives = len(directives.FindAllString(f.template, -1))
	if f.directives == 0 {
		return nil, ErrNoDirectives
	}
	return f, nil
}

// Renders a template with the given data using the [Format] renderer.
func (f *Format) Render(data map[string]any) (_ string, err error) {
	keys := slices.Sorted(maps.Keys(data))
	values := make([]any, 0, len(data))
	for _, key := range keys {
		values = append(values, data[key])
	}

	if len(values) != f.directives {
		return "", fmt.Errorf("template contains %d directives but received %d values", f.directives, len(values))
	}

	return fmt.Sprintf(f.template, values...), nil
}
