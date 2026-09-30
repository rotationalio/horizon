package testenv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Parses text and multimodal model overrides, falling back to fresh defaults when empty.
func TestOpenRouterModelOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		get      func(testing.TB) []string
		defaults []string
	}{
		{
			name:     "text",
			envKey:   "OPENROUTER_TEXT_MODELS",
			get:      OpenRouterTextModels,
			defaults: defaultOpenRouterTextModels,
		},
		{
			name:     "multimodal",
			envKey:   "OPENROUTER_MULTIMODAL_MODELS",
			get:      OpenRouterMultimodalModels,
			defaults: defaultOpenRouterMultimodalModels,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("empty override uses a fresh default slice", func(t *testing.T) {
				t.Setenv(tt.envKey, "")

				models := tt.get(t)
				require.Equal(t, tt.defaults, models)
				models[0] = "mutated/model"
				require.Equal(t, tt.defaults, tt.get(t))
			})

			t.Run("empty model IDs use defaults", func(t *testing.T) {
				t.Setenv(tt.envKey, " , \t, ,, ")
				require.Equal(t, tt.defaults, tt.get(t))
			})

			t.Run("comma-separated overrides are trimmed", func(t *testing.T) {
				t.Setenv(tt.envKey, " model/one, ,model/two ,,  model/three ")
				require.Equal(t, []string{"model/one", "model/two", "model/three"}, tt.get(t))
			})
		})
	}
}
