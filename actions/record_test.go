package actions_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
)

// Records each supported event using one-line helpers and checks participants,
// sequencing, and correlations independently of the resulting transcript.
func TestRecording(t *testing.T) {
	// Prepare a generation and a capability contribution with separate identities.
	var log actions.Actions
	question := prompts.Prompts{message(prompts.RoleUser, "question")}
	call := prompts.ToolCall{CallID: "tool-1", Name: "lookup", Arguments: `{}`}
	result := &prompts.ToolResponse{Content: "found"}
	response := &provider.Response{Model: "model", Output: prompts.Prompts{message(prompts.RoleAssistant, "answer")}}

	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, question)
	log.AddInputGuard(t.Context(), nil)
	correlation := log.AddGenerationRequest(t.Context(), &provider.Request{Model: "model", Input: question})
	log.AddGenerationResponse(t.Context(), correlation, response, nil)
	log.AddOutputGuard(t.Context(), correlation, nil, actions.WithDetail("allowed"))
	log.AddCapabilityRequest(t.Context(), call)
	log.AddCapabilityResponse(t.Context(), call.CallID, result, errors.New("diagnostic"))

	// The same enum identifies both broad participants and specific components.
	participants := [][2]actions.Entity{
		{actions.EntityUser, actions.EntityRunner},
		{actions.EntityInputGuard, actions.EntityRunner},
		{actions.EntityRunner, actions.EntityModel},
		{actions.EntityModel, actions.EntityRunner},
		{actions.EntityOutputGuard, actions.EntityRunner},
		{actions.EntityRunner, actions.EntityCapability},
		{actions.EntityCapability, actions.EntityRunner},
	}
	entries := log.Entries()
	require.Len(t, entries, len(participants))
	require.False(t, log.IsZero())
	require.Equal(t, "generation-1", correlation)

	for i, entry := range entries {
		require.Equal(t, uint64(i+1), entry.Sequence)
		require.False(t, entry.Timestamp.IsZero())
		require.Equal(t, participants[i][0], entry.Sender)
		require.Equal(t, participants[i][1], entry.Receiver)
	}

	require.Equal(t, correlation, entries[3].CorrelationID)
	require.Equal(t, "allowed", entries[4].Detail)
	require.Equal(t, call.CallID, entries[6].CapabilityResponse.CallID)
	require.Equal(t, "diagnostic", entries[6].Error)
	require.Empty(t, result.CallID, "helpers must not mutate caller results")
}

// Capability results and messages sent to the model are recorded separately.
// The message keeps its metadata and tool result even if the caller later edits
// the original. A failed call can be recorded without a tool result.
func TestCapabilityMessages(t *testing.T) {
	contribution := &prompts.Prompt{
		Role:        prompts.RoleTool,
		Type:        prompts.TypeMessage,
		Meta:        map[string]any{"source": "executor"},
		ToolResults: []prompts.ToolResponse{{CallID: "tool-1", Content: "result"}},
	}
	var log actions.Actions

	log.AddCapabilityResponse(t.Context(), "tool-1", &contribution.ToolResults[0], nil)
	log.AddMessage(t.Context(), actions.EntityCapability, actions.EntityModel, prompts.Prompts{nil, contribution}, actions.WithCorrelation("tool-1"), actions.WithDetail("processed"))
	entry := log.Entries()[1]
	require.Equal(t, "processed", entry.Detail)
	require.Equal(t, "tool-1", entry.CorrelationID)
	require.Equal(t, contribution.ToolResults[0], *log.Entries()[0].CapabilityResponse)
	require.Equal(t, prompts.Prompts{nil, contribution}, log.Messages())

	// Changing the original prompt must not change the recorded tool result.
	contribution.ToolResults[0].Content = "changed"
	require.Equal(t, "result", log.Messages()[1].ToolResults[0].Content)

	// A failed call still gets a response event, even without a tool result.
	log.AddCapabilityResponse(t.Context(), "tool-2", nil, errors.New("failed"))
	require.Equal(t, 3, log.Len())
	require.Nil(t, log.Entries()[2].CapabilityResponse)
	require.Equal(t, "failed", log.Entries()[2].Error)
}
