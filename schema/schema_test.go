package schema_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// Verifies JSON schemas decode to objects and invalid or missing definitions fail cleanly.
func TestSchemaObject(t *testing.T) {
	tests := []struct {
		name      string
		input     *schema.Schema
		want      map[string]any
		wantError bool
		wantErr   error
	}{
		{
			name: "nil schema defaults to object",
			want: map[string]any{"type": "object"},
		},
		{
			name: "object schema",
			input: &schema.Schema{
				MimeType: mime.ApplicationSchemaJSON,
				Data:     `{"type":"object","properties":{"name":{"type":"string"}}}`,
			},
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:      "invalid JSON",
			input:     &schema.Schema{Data: `{"type":`},
			wantError: true,
		},
		{
			name:      "non-object JSON",
			input:     &schema.Schema{Data: `[]`},
			wantError: true,
		},
		{
			name:    "null JSON",
			input:   &schema.Schema{Data: `null`},
			wantErr: errors.ErrInvalidJSONSchema,
		},
		{
			name:    "missing schema data",
			input:   &schema.Schema{},
			wantErr: errors.ErrNoJSONSchema,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: decode the schema into its object representation.
			got, err := tt.input.Object()

			// Assert: verify either the converted object or the expected failure.
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// Schema.Clone returns an equal value with independent pointer identity.
func TestSchemaClone(t *testing.T) {
	t.Run("copies fields independently", func(t *testing.T) {
		// Setup: populate every schema field before taking a value snapshot.
		original := &schema.Schema{
			Name:        "output",
			Description: "Generated output",
			Strict:      true,
			MimeType:    mime.ApplicationSchemaJSON,
			Version:     semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: "rc.1"},
			URI:         "https://example.com/schema.json",
			Data:        `{"type":"object"}`,
		}
		wantOriginal := *original

		// Execute: clone, then edit the clone's scalar fields.
		cloned := original.Clone()
		require.NotSame(t, original, cloned)
		require.Equal(t, wantOriginal, *cloned)
		cloned.Name = "changed"
		cloned.Description = "changed"
		cloned.Strict = false
		cloned.MimeType = "text/plain"
		cloned.Version.Major = 9
		cloned.URI = "https://example.com/changed.json"
		cloned.Data = `{"type":"string"}`

		// Assert: no clone mutation changed the source schema.
		require.Equal(t, wantOriginal, *original)
	})

	t.Run("nil schema", func(t *testing.T) {
		var absent *schema.Schema
		require.Nil(t, absent.Clone())
	})
}
