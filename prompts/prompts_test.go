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

// TestPromptTypeIsKnown distinguishes supported prompt types from the unknown
// sentinel and invalid values.
func TestPromptTypeIsKnown(t *testing.T) {
	require.False(t, prompts.TypeUnknown.IsKnown())
	require.True(t, prompts.TypeMessage.IsKnown())
	require.False(t, prompts.Type(255).IsKnown())
}

// TestPromptRoleIsKnown distinguishes supported prompt roles from the unknown
// sentinel and invalid values.
func TestPromptRoleIsKnown(t *testing.T) {
	require.False(t, prompts.RoleUnknown.IsKnown())
	require.True(t, prompts.RoleAssistant.IsKnown())
	require.False(t, prompts.Role(255).IsKnown())
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
		name        string
		typeFilter  []prompts.Type
		wantIndexes []int
	}{
		{
			name:        "no exclusions",
			wantIndexes: []int{0, 1, 2, 3, 4, 5, 6},
		},
		{
			name:        "reasoning",
			typeFilter:  []prompts.Type{prompts.TypeReasoning},
			wantIndexes: []int{0, 1, 2, 4, 5, 6},
		},
		{
			name:        "reasoning and unknown",
			typeFilter:  []prompts.Type{prompts.TypeReasoning, prompts.TypeUnknown},
			wantIndexes: []int{1, 2, 4, 5, 6},
		},
		{
			name:        "multiple and duplicate exclusions",
			typeFilter:  []prompts.Type{prompts.TypeReasoning, prompts.TypeToolCall, prompts.TypeReasoning},
			wantIndexes: []int{0, 1, 2, 5, 6},
		},
		{
			name:        "all known types",
			typeFilter:  []prompts.Type{prompts.TypeUnknown, prompts.TypeMessage, prompts.TypeReasoning, prompts.TypeToolCall, prompts.TypeRefusal, prompts.TypeContentFilter},
			wantIndexes: []int{2},
		},
		{
			name:        "unmatched exclusion",
			typeFilter:  []prompts.Type{prompts.Type(255)},
			wantIndexes: []int{0, 1, 2, 3, 4, 5, 6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: filtering must not edit the original slice.
			got := original.FilterOutTypes(tt.typeFilter...)

			// Assert retained pointers and order, including nil entries.
			require.Len(t, got, len(tt.wantIndexes))
			for i, index := range tt.wantIndexes {
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

// TestPromptsFilterOutRoles verifies role filtering preserves order, nil entries, and the source slice.
func TestPromptsFilterOutRoles(t *testing.T) {
	// Interleave every role with a nil entry to check filtering keeps retained entries in order.
	original := prompts.Prompts{
		{Role: prompts.RoleUnknown},
		{Role: prompts.RoleUser},
		nil,
		{Role: prompts.RoleSystem},
		{Role: prompts.RoleDeveloper},
		{Role: prompts.RoleAssistant},
		{Role: prompts.RoleTool},
		{Role: prompts.RoleFunction},
	}
	tests := []struct {
		name        string
		roleFilter  []prompts.Role
		wantIndexes []int
	}{
		{
			name:        "no exclusions",
			wantIndexes: []int{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:        "assistant",
			roleFilter:  []prompts.Role{prompts.RoleAssistant},
			wantIndexes: []int{0, 1, 2, 3, 4, 6, 7},
		},
		{
			name:        "assistant and tool",
			roleFilter:  []prompts.Role{prompts.RoleAssistant, prompts.RoleTool},
			wantIndexes: []int{0, 1, 2, 3, 4, 7},
		},
		{
			name:        "multiple and duplicate exclusions",
			roleFilter:  []prompts.Role{prompts.RoleUser, prompts.RoleFunction, prompts.RoleUser},
			wantIndexes: []int{0, 2, 3, 4, 5, 6},
		},
		{
			name:        "all roles",
			roleFilter:  []prompts.Role{prompts.RoleUnknown, prompts.RoleUser, prompts.RoleSystem, prompts.RoleDeveloper, prompts.RoleAssistant, prompts.RoleTool, prompts.RoleFunction},
			wantIndexes: []int{2},
		},
		{
			name:        "unmatched exclusion",
			roleFilter:  []prompts.Role{prompts.Role(255)},
			wantIndexes: []int{0, 1, 2, 3, 4, 5, 6, 7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Filter without editing the input, then verify retained pointers and order.
			got := original.FilterOutRoles(tt.roleFilter...)
			require.Len(t, got, len(tt.wantIndexes))
			for i, index := range tt.wantIndexes {
				require.Same(t, original[index], got[i])
			}

			// Replacing a result entry must not replace an entry in the original slice.
			got[0] = &prompts.Prompt{Content: "replacement"}
			require.Empty(t, original[0].Content)
			require.Nil(t, original[2])
		})
	}

	// Nil and empty prompt slices retain their established behavior.
	t.Run("nil and empty", func(t *testing.T) {
		var empty prompts.Prompts
		require.Nil(t, empty.FilterOutRoles(prompts.RoleAssistant))
		require.NotNil(t, prompts.Prompts{}.FilterOutRoles(prompts.RoleAssistant))
		require.Empty(t, prompts.Prompts{{Role: prompts.RoleAssistant}}.FilterOutRoles(prompts.RoleAssistant))
	})
}
