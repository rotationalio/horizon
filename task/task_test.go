package task_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/ulid"
	"gopkg.in/yaml.v3"
)

// Verifies execution copies isolate task configuration while preserving nil fields.
func TestTaskClone(t *testing.T) {
	t.Run("mutable configuration", func(t *testing.T) {
		original := &task.Task{
			Model:        task.Model{Parameters: params.New(map[string]any{"temperature": 0.5})},
			Input:        &schema.Input{Schema: &schema.Schema{Name: "input"}},
			Output:       &schema.Output{Schema: &schema.Schema{Name: "output"}},
			Prompts:      prompts.Templates{nil, {Content: "original"}},
			Capabilities: []capabilities.Name{"original"},
			Provider: &provider.Config{
				DefaultModel: "original-model",
				Credentials:  auth.NewNone(),
			},
		}
		cloned := original.Clone()
		require.Equal(t, original, cloned)
		require.NotSame(t, original, cloned)
		cloned.Model.Parameters.Set("temperature", 1.0)
		cloned.Input.Schema.Name = "changed input"
		cloned.Output.Schema.Name = "changed output"
		cloned.Prompts[1].Content = "changed prompt"
		cloned.Capabilities[0] = "changed"
		cloned.Provider.DefaultModel = "changed-model"
		cloned.Provider.Credentials = auth.NewAPIKey("new-key")
		require.NotSame(t, original.Provider, cloned.Provider)
		require.Equal(t, "original-model", original.Provider.DefaultModel)
		require.Equal(t, auth.TypeNone, original.Provider.Credentials.Type())
		require.Equal(t, "input", original.Input.Schema.Name)
		require.Equal(t, "output", original.Output.Schema.Name)
		require.Equal(t, "original", original.Prompts[1].Content)
		require.Equal(t, capabilities.Name("original"), original.Capabilities[0])
		value, ok := original.Model.Parameters.Get("temperature")
		require.True(t, ok)
		require.Equal(t, 0.5, value)
	})

	t.Run("nil task", func(t *testing.T) {
		var original *task.Task
		require.Nil(t, original.Clone())
	})

	t.Run("empty task", func(t *testing.T) {
		original := &task.Task{}
		require.Equal(t, original, original.Clone())
	})

	t.Run("nil schemas", func(t *testing.T) {
		original := &task.Task{Input: &schema.Input{}, Output: &schema.Output{}}
		cloned := original.Clone()
		require.Nil(t, cloned.Input.Schema)
		require.Nil(t, cloned.Output.Schema)
		require.NotSame(t, original.Input, cloned.Input)
		require.NotSame(t, original.Output, cloned.Output)
	})
}

// Verifies task provider configuration and built-in credentials round-trip in files.
func TestTaskSerialization(t *testing.T) {
	id := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	for _, format := range []string{"JSON", "YAML"} {
		t.Run(format, func(t *testing.T) {
			for _, selected := range []bool{false, true} {
				name := "unselected"
				if selected {
					name = "selected"
				}
				t.Run(name, func(t *testing.T) {
					original := task.Task{
						ID:      id,
						Model:   task.Model{Slug: "model"},
						Output:  &schema.Output{Modality: modality.Text},
						Prompts: prompts.Templates{},
					}
					if selected {
						original.Provider = &provider.Config{
							ID:           id,
							APIType:      provider.APITypeMock,
							ProviderType: provider.ProviderTypeMock,
							DefaultModel: "model",
							Credentials:  auth.NewAPIKey("exported-secret"),
						}

					}
					var data []byte
					var err error
					var decoded task.Task
					if format == "JSON" {
						data, err = json.Marshal(original)
						require.NoError(t, err)
						err = json.Unmarshal(data, &decoded)
					} else {
						data, err = yaml.Marshal(original)
						require.NoError(t, err)
						err = yaml.Unmarshal(data, &decoded)
					}
					require.NoError(t, err)
					require.Equal(t, original, decoded)
					if selected {
						require.Contains(t, string(data), "provider")
						require.Contains(t, string(data), "exported-secret")
					} else {
						require.NotContains(t, string(data), "credentials")
					}
				})
			}
		})
	}
}
