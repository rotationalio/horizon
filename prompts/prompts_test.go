package prompts_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/prompts"
)

// Clone copies prompt-owned mutable fields while preserving nil entries and order.
func TestPromptClone(t *testing.T) {
	// Setup: use every mutable prompt field and nested JSON-shaped metadata.
	original := &prompts.Prompt{
		Content:     "question",
		Citations:   []prompts.Citation{{Title: "source"}},
		Meta:        map[string]any{"nested": map[string]any{"labels": []any{"original"}}},
		ToolCalls:   []prompts.ToolCall{{CallID: "call", Arguments: `{}`}},
		ToolResults: []prompts.ToolResponse{{CallID: "prior", Content: "result"}},
	}
	messages := prompts.Prompts{original, nil}

	// Execute both clone methods, then mutate each cloned container.
	cloned := original.Clone()
	cloned.Content = "changed"
	cloned.Citations[0].Title = "changed"
	cloned.Meta["nested"].(map[string]any)["labels"].([]any)[0] = "changed"
	cloned.ToolCalls[0].Arguments = `{"changed":true}`
	cloned.ToolResults[0].Content = "changed"
	clonedMessages := messages.Clone()
	clonedMessages[0].Content = "slice changed"

	// Assert neither clone aliases the source prompt or its mutable fields.
	require.NotSame(t, original, cloned)
	require.Equal(t, "question", original.Content)
	require.Equal(t, "source", original.Citations[0].Title)
	require.Equal(t, "original", original.Meta["nested"].(map[string]any)["labels"].([]any)[0])
	require.Equal(t, `{}`, original.ToolCalls[0].Arguments)
	require.Equal(t, "result", original.ToolResults[0].Content)
	require.Equal(t, "slice changed", clonedMessages[0].Content)
	require.Nil(t, clonedMessages[1])

	// Nil values remain nil, and empty collections remain independent collections.
	var absent *prompts.Prompt
	var noMessages prompts.Prompts
	require.Nil(t, absent.Clone())
	require.Nil(t, noMessages.Clone())
	require.NotNil(t, prompts.Prompts{}.Clone())
}

func TestPromptsFilterOutTypes(t *testing.T) {
	// Setup: interleave every known type with nil to verify order and nil handling.
	original := prompts.Prompts{
		{Type: prompts.TypeUnknown},
		{Type: prompts.TypeMessage},
		nil,
		{Type: prompts.TypeReasoning},
		{Type: prompts.TypeToolCall},
		{Type: prompts.TypeRefusal},
		{Type: prompts.TypeContentFilter},
	}
	tests := []struct {
		name    string
		types   []prompts.Type
		indexes []int
	}{
		{
			name:    "no exclusions",
			indexes: []int{0, 1, 2, 3, 4, 5, 6},
		},
		{
			name:    "reasoning",
			types:   []prompts.Type{prompts.TypeReasoning},
			indexes: []int{0, 1, 2, 4, 5, 6},
		},
		{
			name:    "reasoning and unknown",
			types:   []prompts.Type{prompts.TypeReasoning, prompts.TypeUnknown},
			indexes: []int{1, 2, 4, 5, 6},
		},
		{
			name:    "multiple and duplicate exclusions",
			types:   []prompts.Type{prompts.TypeReasoning, prompts.TypeToolCall, prompts.TypeReasoning},
			indexes: []int{0, 1, 2, 5, 6},
		},
		{
			name:    "all known types",
			types:   []prompts.Type{prompts.TypeUnknown, prompts.TypeMessage, prompts.TypeReasoning, prompts.TypeToolCall, prompts.TypeRefusal, prompts.TypeContentFilter},
			indexes: []int{2},
		},
		{
			name:    "unmatched exclusion",
			types:   []prompts.Type{prompts.Type(255)},
			indexes: []int{0, 1, 2, 3, 4, 5, 6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: filtering must not edit the original slice.
			got := original.FilterOutTypes(tt.types...)

			// Assert retained pointers and order, including nil entries.
			require.Len(t, got, len(tt.indexes))
			for i, index := range tt.indexes {
				require.True(t, original[index] == got[i])
			}

			// Replacing a returned entry must not replace the original entry.
			got[0] = &prompts.Prompt{Content: "replacement"}
			require.Empty(t, original[0].Content)
			require.Nil(t, original[2])
		})
	}

	t.Run("nil and empty", func(t *testing.T) {
		var empty prompts.Prompts
		require.Nil(t, empty.FilterOutTypes(prompts.TypeReasoning))
		require.NotNil(t, prompts.Prompts{}.FilterOutTypes(prompts.TypeReasoning))
		require.Empty(t, prompts.Prompts{{Type: prompts.TypeReasoning}}.FilterOutTypes(prompts.TypeReasoning))
	})
}
