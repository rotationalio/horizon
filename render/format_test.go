package render_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/render"
)

func TestFormatRender(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		testCases := []struct {
			template string
			data     map[string]any
			expected string
		}{
			{template: "hello world %d times!", data: map[string]any{"count": 42}, expected: "hello world 42 times!"},
			{template: "%s %s %s", data: map[string]any{"a": "alpha", "B": "bravo", "c": "charlie"}, expected: "bravo alpha charlie"},
		}

		for i, tc := range testCases {
			renderer, err := render.NewFormat(tc.template)
			require.NoError(t, err, "expected valid template to create renderer for test case %d", i)

			rendered, err := renderer.Render(tc.data)
			require.NoError(t, err, "expected valid rendered template for test case %d", i)
			require.Equal(t, tc.expected, rendered, "expected rendered template to match for test case %d", i)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		// TODO: these test cases need to be fixed, the regular expression is not working.
		testCases := []string{
			"",
			"hello world",
			// "hello world: %%q",
			// "not a % directive",
			// "an escaped %% percent sign",
		}

		for i, tc := range testCases {
			renderer, err := render.NewFormat(tc)
			require.ErrorIs(t, err, render.ErrNoDirectives, "expected error for test case %d", i)
			require.Nil(t, renderer, "expected nil renderer for test case %d", i)
		}
	})

	t.Run("Errors", func(t *testing.T) {
		testCases := []struct {
			template string
			data     map[string]any
			expected string
		}{
			{template: "hello world: %q", data: nil, expected: "template contains 1 directives but received 0 values"},
			{template: "hello world: %q", data: map[string]any{}, expected: "template contains 1 directives but received 0 values"},
			{template: "%s %s %s", data: map[string]any{"a": "alpha", "B": "bravo"}, expected: "template contains 3 directives but received 2 values"},
			{template: "%s %s %s", data: map[string]any{"a": "alpha", "B": "bravo", "c": "charlie", "d": "delta"}, expected: "template contains 3 directives but received 4 values"},
		}

		for _, tc := range testCases {
			renderer, err := render.NewFormat(tc.template)
			require.NoError(t, err, "expected valid template to create renderer")

			rendered, err := renderer.Render(tc.data)
			require.EqualError(t, err, tc.expected)
			require.Empty(t, rendered, "expected empty rendered template")
		}
	})
}
