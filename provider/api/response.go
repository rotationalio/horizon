package api

import (
	"reflect"
	"time"

	"go.rtnl.ai/horizon/capabilities"
)

// Standardizes the allowed response type for all LLM clients and providers.
type Response struct {
	ID          string
	Output      []Output
	Attachments []*Attachment
	Created     time.Time
	Model       string
	Usage       Usage
	Meta        Meta
	ToolCalls   []capabilities.ToolCall
}

// Represents the Model output that is returned to the user.
type Output struct {
	ID        string
	Role      Role
	Status    string
	Type      OutputType
	Content   string
	Citations []Citation
	Meta      Meta
}

// A citation is a URL citation from the Model response.
type Citation struct {
	StartIndex  int64
	EndIndex    int64
	Title       string
	URL         string
	Description string
}

// Meta is a map of key-value pairs that have additional metadata about the output.
type Meta map[string]any

// Puts a string value into the meta data if it is a string  and it is not empty.
func (m Meta) PutString(key string, value any) {
	if s, ok := underlyingString(value); ok && s != "" {
		m[key] = s
	}
}

// Returns the underlying string value if the value is a string or a pointer to
// a string and it is not nil, otherwise returns an empty string and false.
func underlyingString(value any) (_ string, wasString bool) {
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.String {
		return rv.String(), true
	}

	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return underlyingString(rv.Elem().Interface())
	}

	return "", false
}
