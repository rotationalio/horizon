package prompts_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/horizon/prompts"
)

func TestContext(t *testing.T) {
	t.Run("IsText", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			c := Context{DefaultContextKey: "test"}
			require.True(t, c.IsText())
		})

		t.Run("False", func(t *testing.T) {
			testCases := []Context{
				{},
				{"foo": "bar"},
				{DefaultContextKey: "foo", "color": "red"},
				{DefaultContextKey: 42},
			}

			for _, tc := range testCases {
				require.False(t, tc.IsText())
			}
		})
	})

	t.Run("JSON", func(t *testing.T) {
		t.Run("Text", func(t *testing.T) {
			val := `"hello world"`

			var c Context
			require.NoError(t, json.Unmarshal([]byte(val), &c))
			require.Equal(t, Context{DefaultContextKey: "hello world"}, c)

			out, err := json.Marshal(c)
			require.NoError(t, err)
			require.Equal(t, val, string(out))
		})

		t.Run("Object", func(t *testing.T) {
			val := `{"color": "red", "foo": "bar"}`

			var c Context
			require.NoError(t, json.Unmarshal([]byte(val), &c))
			require.Equal(t, Context{"foo": "bar", "color": "red"}, c)

			out, err := json.Marshal(c)
			require.NoError(t, err)
			require.JSONEq(t, val, string(out))
		})

	})
}
