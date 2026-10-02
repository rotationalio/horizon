package capabilities

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
