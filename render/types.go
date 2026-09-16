package render

//go:generate enumify -names typeNames

// [Template] [Renderer] type.
type Type uint8

const (
	Unknown Type = iota
	// Uses C-style formatting directives with [fmt.Sprintf].
	Sprintf
	// TODO: Uses Jinja-style formatting directives.
	Jinja
	// Uses Go template engine with [text.Template].
	Go
)

var typeNames = []string{
	"unknown",
	"sprintf",
	"jinja",
	"go",
}
