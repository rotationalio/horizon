package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"
)

type stringAlias string

// Verifies non-empty strings, defined string types, and string pointers are
// normalized while empty and non-string values are omitted.
func TestMetaPutString(t *testing.T) {
	meta := make(provider.Meta)
	alias := stringAlias("alias")

	meta.PutString("string", "value")
	meta.PutString("alias", alias)
	meta.PutString("pointer", &alias)
	meta.PutString("empty", "")
	meta.PutString("integer", 42)
	meta.PutString("nil", nil)

	require.Equal(t, provider.Meta{
		"string":  "value",
		"alias":   "alias",
		"pointer": "alias",
	}, meta)
}

// Request.Clone isolates each mutable provider request field while preserving nil values.
func TestRequestClone(t *testing.T) {
	t.Run("mutable request fields", func(t *testing.T) {
		// Setup: populate every nested request type with mutable data.
		original := &provider.Request{
			Model:  "model-original",
			Params: params.New(map[string]any{params.Temperature: 0.5}),
			Input: prompts.Prompts{{
				Role:        prompts.RoleUser,
				Content:     "question",
				Citations:   []prompts.Citation{{Title: "source"}},
				Meta:        map[string]any{"nested": map[string]any{"labels": []any{"original"}}},
				ToolCalls:   []prompts.ToolCall{{CallID: "call", Arguments: `{}`}},
				ToolResults: []prompts.ToolResponse{{CallID: "prior", Content: "result"}},
			}},
			Attachments: attachments.Attachments{{Filename: "input.txt", Data: []byte("original")}},
			OutputSchema: &schema.Schema{
				Name: "output",
				Data: `{"type":"object"}`,
			},
			Tools: []capabilities.Definition{{
				Name:        "lookup",
				Description: "Lookup data",
				InputSchema: &schema.Schema{Name: "lookup-input", Data: `{"type":"object"}`},
			}},
		}

		// Execute: clone the request, then mutate every cloned nested container.
		cloned := original.Clone()
		require.Equal(t, original, cloned)
		require.NotSame(t, original, cloned)
		require.NotSame(t, original.Input[0], cloned.Input[0])
		require.NotSame(t, original.Attachments[0], cloned.Attachments[0])
		require.NotSame(t, original.OutputSchema, cloned.OutputSchema)
		require.NotSame(t, original.Tools[0].InputSchema, cloned.Tools[0].InputSchema)

		cloned.Model = "model-cloned"
		cloned.Params.Set(params.Temperature, 1.0)
		cloned.Input[0].Content = "changed question"
		cloned.Input[0].Citations[0].Title = "changed source"
		cloned.Input[0].Meta["nested"].(map[string]any)["labels"].([]any)[0] = "changed"
		cloned.Input[0].ToolCalls[0].Arguments = `{"changed":true}`
		cloned.Input[0].ToolResults[0].Content = "changed result"
		cloned.Attachments[0].Filename = "changed.txt"
		cloned.Attachments[0].Data[0] = 'X'
		cloned.OutputSchema.Data = `{"type":"string"}`
		cloned.Tools[0].Name = "changed"
		cloned.Tools[0].InputSchema.Data = `{"type":"string"}`

		// Assert: all mutations stayed within the clone.
		require.Equal(t, "model-original", original.Model)
		value, ok := original.Params.Get(params.Temperature)
		require.True(t, ok)
		require.Equal(t, 0.5, value)
		require.Equal(t, "question", original.Input[0].Content)
		require.Equal(t, "source", original.Input[0].Citations[0].Title)
		require.Equal(t, "original", original.Input[0].Meta["nested"].(map[string]any)["labels"].([]any)[0])
		require.Equal(t, `{}`, original.Input[0].ToolCalls[0].Arguments)
		require.Equal(t, "result", original.Input[0].ToolResults[0].Content)
		require.Equal(t, "input.txt", original.Attachments[0].Filename)
		require.Equal(t, "original", string(original.Attachments[0].Data))
		require.Equal(t, `{"type":"object"}`, original.OutputSchema.Data)
		require.Equal(t, capabilities.Name("lookup"), original.Tools[0].Name)
		require.Equal(t, `{"type":"object"}`, original.Tools[0].InputSchema.Data)
	})

	t.Run("nil and empty requests", func(t *testing.T) {
		var absent *provider.Request
		require.Nil(t, absent.Clone())

		empty := &provider.Request{}
		cloned := empty.Clone()
		require.Equal(t, empty, cloned)
		require.NotSame(t, empty, cloned)
	})
}
