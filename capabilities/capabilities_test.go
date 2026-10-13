package capabilities

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/schema"
)

// Validates that the Name type validates correctly.
func TestNameValidate(t *testing.T) {
	t.Run("valid names", func(t *testing.T) {
		for _, name := range []Name{"a", Name(strings.Repeat("a", 64)), "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"} {
			require.NoError(t, name.Validate())
		}
	})

	t.Run("invalid names", func(t *testing.T) {
		for _, name := range []Name{"", Name(strings.Repeat("a", 65)), "é", " ", ":", ";", ",", "<", ".", ">", "?", "/", "\\", "\"", "'", "!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "[", "]", "{", "}", "+", "=", "`", "~"} {
			require.ErrorIs(t, name.Validate(), ErrInvalidName)
		}
	})
}

// Definition.Clone copies the schema so a caller can safely edit its clone.
func TestDefinitionClone(t *testing.T) {
	// Setup: attach a mutable schema to a capability definition.
	original := Definition{
		Name:        "lookup",
		Description: "Find a record",
		InputSchema: &schema.Schema{Name: "lookup-input", Data: `{"type":"object"}`},
	}

	// Execute: clone the definition and change the cloned schema.
	cloned := original.Clone()
	require.Equal(t, original, cloned)
	require.NotSame(t, original.InputSchema, cloned.InputSchema)
	cloned.InputSchema.Name = "changed"
	cloned.InputSchema.Data = `{"type":"string"}`

	// Assert: scalar fields are preserved and schema changes do not reach the source.
	require.Equal(t, Name("lookup"), original.Name)
	require.Equal(t, "Find a record", original.Description)
	require.Equal(t, "lookup-input", original.InputSchema.Name)
	require.Equal(t, `{"type":"object"}`, original.InputSchema.Data)
	require.Nil(t, (Definition{Name: "without-schema"}).Clone().InputSchema)
}
