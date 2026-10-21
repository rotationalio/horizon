package provider

import (
	"reflect"
	"time"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/metadata"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/schema"
)

// Combines task configuration and rendered input for one inference.
type Request struct {
	Model        string                    // The model name sent directly to the provider.
	Params       *params.Params            // Provider-neutral generation parameters.
	Input        prompts.Prompts           // Rendered prompts sent to the model.
	Attachments  attachments.Attachments   // Files, images, links, and other attached input.
	OutputSchema *schema.Schema            // Requested output schema, particularly for JSON.
	Tools        []capabilities.Definition // Tools advertised to the model.
}

// Clone returns a request snapshot with independent containers and typed values.
// Opaque values inside parameters and prompt metadata follow their own clone rules.
func (r *Request) Clone() *Request {
	if r == nil {
		return nil
	}

	cloned := *r
	cloned.Params = r.Params.Clone()
	cloned.Input = r.Input.Clone()
	cloned.Attachments = r.Attachments.Clone()
	cloned.OutputSchema = r.OutputSchema.Clone()
	if r.Tools != nil {
		cloned.Tools = make([]capabilities.Definition, len(r.Tools))
		for i, tool := range r.Tools {
			cloned.Tools[i] = tool.Clone()
		}
	}
	return &cloned
}

// Represents the provider-neutral result of an inference request.
type Response struct {
	ID          string
	Model       string
	Created     time.Time
	Usage       Usage
	Output      prompts.Prompts
	Attachments attachments.Attachments
	Meta        Meta
}

// Clone returns a response snapshot with independent prompts and attachments.
// Metadata uses the same cloning rules as prompt metadata: JSON-shaped containers
// are copied recursively; opaque values retain their existing ownership rules.
func (r *Response) Clone() *Response {
	if r == nil {
		return nil
	}

	cloned := *r
	cloned.Output = r.Output.Clone()
	cloned.Attachments = r.Attachments.Clone()
	cloned.Meta = Meta(metadata.Metadata(r.Meta).Clone())

	return &cloned
}

// Stores supplemental key-value data attached to provider responses and output
// prompts.
type Meta map[string]any

// Stores a non-empty underlying string value. String-based defined types and
// non-nil pointers to them are normalized to plain strings; all other values
// are ignored.
func (m Meta) PutString(key string, value any) {
	if s, ok := underlyingString(value); ok && s != "" {
		m[key] = s
	}
}

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

// Reports token consumption and provider cost when available.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	APICost      float64
}
