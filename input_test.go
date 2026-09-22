package horizon_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/prompts"
)

// TestContext verifies the prompt context's text shortcut and JSON wire format,
// including the distinction between a single default text value and an object
// containing arbitrary context fields.
func TestContext(t *testing.T) {
	t.Run("IsText", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			c := prompts.Context{prompts.DefaultContextKey: "test"}
			require.True(t, c.IsText())
		})

		t.Run("False", func(t *testing.T) {
			testCases := []prompts.Context{
				{},
				{"foo": "bar"},
				{prompts.DefaultContextKey: "foo", "color": "red"},
				{prompts.DefaultContextKey: 42},
			}

			for _, tc := range testCases {
				require.False(t, tc.IsText())
			}
		})
	})

	t.Run("JSON", func(t *testing.T) {
		t.Run("Text", func(t *testing.T) {
			val := `"hello world"`

			var c prompts.Context
			require.NoError(t, json.Unmarshal([]byte(val), &c))
			require.Equal(t, prompts.Context{prompts.DefaultContextKey: "hello world"}, c)

			out, err := json.Marshal(c)
			require.NoError(t, err)
			require.Equal(t, val, string(out))
		})

		t.Run("Object", func(t *testing.T) {
			val := `{"color": "red", "foo": "bar"}`

			var c prompts.Context
			require.NoError(t, json.Unmarshal([]byte(val), &c))
			require.Equal(t, prompts.Context{"foo": "bar", "color": "red"}, c)

			out, err := json.Marshal(c)
			require.NoError(t, err)
			require.JSONEq(t, val, string(out))
		})
	})
}
