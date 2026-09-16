package schema

import (
	"encoding/json"
	"fmt"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// Defines a schema for an input or output of a task based on the MIME type.
// The schema can be a JSON Schema, a YAML Schema, a XML Schema, etc.
type Schema struct {
	Name        string         `json:"name,omitempty" yaml:"name,omitempty" msg:"name,omitempty"`                      // The name of the schema
	Description string         `json:"description,omitempty" yaml:"description,omitempty" msg:"description,omitempty"` // The description of the schema
	Strict      bool           `json:"strict,omitempty" yaml:"strict,omitempty" msg:"strict,omitempty"`                // Whether to enable strict schema adherence when generating the output
	MimeType    mime.Type      `json:"mimetype" yaml:"mimetype" msg:"mimetype"`                                        // The MIME type of the schema
	Version     semver.Version `json:"version" yaml:"version" msg:"version"`                                           // The version of the schema
	URI         string         `json:"uri,omitempty" yaml:"uri,omitempty" msg:"uri,omitempty"`                         // The URI of the schema definition (usually JSON)
	Data        string         `json:"data,omitempty" yaml:"data,omitempty" msg:"data,omitempty"`                      // The schema definition (usually JSON)
}

// TODO: Add support for schema validation directly in the Horizon package.
func Validate(data any) error {
	return nil
}

// Returns the correct schema format for the [Schema.MimeType].
func (s *Schema) Schema() (any, error) {
	// TODO: handle other schema types if necessary
	switch s.MimeType {
	case mime.ApplicationSchemaJSON:
		return s.JSON()
	default:
		return nil, fmt.Errorf("unsupported schema type: %s", s.MimeType)
	}
}

func (s *Schema) JSON() (_ any, err error) {
	// TODO: cache the jsonschema-go resolved schema object on the Schema struct.
	// Check if there is any schema data on the struct. If not, fetch it from the URI.
	if s.Data == "" {
		if s.URI == "" {
			return nil, errors.ErrNoJSONSchema
		}

		// TODO: Fetch the schema from the URI.
		return nil, errors.New("remote schema fetch not implemented yet")
	}

	// TODO: return a resolved and validated JSON schema object from jsonschema-go
	return json.RawMessage(s.Data), nil
}
