package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	var templates int
	factory := func(t Type, template string) (Renderer, error) {
		templates++
		return New(t, template)
	}

	cache, err := NewCache(4, factory)
	require.NoError(t, err, "could not create cache")

	alpha := "%s is %d years old"
	bravo := "greetings %s, take me to your leader"

	for i := 0; i < 64; i++ {
		rendered, err := cache.Render(Sprintf, alpha, map[string]any{"1": "John", "2": 30})
		require.NoError(t, err, "could not render template")
		require.Equal(t, "John is 30 years old", rendered, "expected rendered template to match")

		for i := 0; i < 4; i++ {
			rendered, err := cache.Render(Sprintf, bravo, map[string]any{"1": "John"})
			require.NoError(t, err, "could not render template")
			require.Equal(t, "greetings John, take me to your leader", rendered, "expected rendered template to match")
		}
	}

	// The factory should have been called 2 times, once for the alpha template and once for the bravo template.
	require.Equal(t, 2, templates, "expected 2 templates to be created")
}
