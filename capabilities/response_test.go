package capabilities_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
)

// Checks that the convenience constructor creates and retains a valid correlated prompt.
func TestNewToolResponsePrompt(t *testing.T) {
	response := capabilities.NewToolResponse("call-123", "done")
	require.Equal(t, "call-123", response.CallID())
	require.NoError(t, response.Validate())

	message := response.ToolPrompt()
	require.Same(t, message, response.ToolPrompt())
	require.Equal(t, prompts.RoleTool, message.Role)
	require.Empty(t, message.Content)
	require.Equal(t, prompts.TypeMessage, message.Type)
	require.Equal(t, []prompts.ToolResponse{{CallID: "call-123", Content: "done"}}, message.ToolResults)
}

// Covers valid responses and each prompt invariant enforced by validation.
func TestToolResponseValidate(t *testing.T) {
	tests := []struct {
		name     string
		response capabilities.ToolResponse
		fields   []string
	}{
		{
			name:     "missing call ID",
			response: capabilities.NewToolResponse("", "done"),
			fields:   []string{"call_id"},
		},
		{
			name:     "missing prompt",
			response: capabilities.NewTestToolResponse("call-1", nil),
			fields:   []string{"tool_prompt"},
		},
		{
			name: "wrong role",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role:        prompts.RoleUser,
				Type:        prompts.TypeMessage,
				ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "done"}},
			}),
			fields: []string{"role"},
		},
		{
			name: "wrong prompt type",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role:        prompts.RoleTool,
				Type:        prompts.TypeToolCall,
				ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "done"}},
			}),
			fields: []string{"type"},
		},
		{
			name: "no tool results",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role: prompts.RoleTool,
				Type: prompts.TypeMessage,
			}),
			fields: []string{"tool_results"},
		},
		{
			name: "multiple tool results",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role: prompts.RoleTool,
				Type: prompts.TypeMessage,
				ToolResults: []prompts.ToolResponse{
					{CallID: "call-1", Content: "one"},
					{CallID: "call-2", Content: "two"},
				},
			}),
			fields: []string{"tool_results"},
		},
		{
			name: "mismatched result call ID",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role:        prompts.RoleTool,
				Type:        prompts.TypeMessage,
				ToolResults: []prompts.ToolResponse{{CallID: "call-2", Content: "done"}},
			}),
			fields: []string{"tool_results.call_id"},
		},
		{
			name: "prompt content duplicates tool result",
			response: capabilities.NewTestToolResponse("call-1", &prompts.Prompt{
				Role:        prompts.RoleTool,
				Content:     "done",
				Type:        prompts.TypeMessage,
				ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "done"}},
			}),
			fields: []string{"content"},
		},
		{
			name:     "valid response",
			response: capabilities.NewToolResponse("call-1", "done"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if len(tt.fields) == 0 {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, capabilities.ErrInvalidToolPrompt)
			errors.RequireValidationFields(t, err, tt.fields...)
		})
	}
}

// Covers grouping, nil inputs, duplicate call IDs, and invalid responses.
func TestMergeToolResponses(t *testing.T) {
	tests := []struct {
		name          string
		responses     []capabilities.ToolResponse
		wantResults   []prompts.ToolResponse
		wantNilPrompt bool
		wantError     string
		wantFields    []string
	}{
		{
			name: "merges multiple responses in order",
			responses: []capabilities.ToolResponse{
				capabilities.NewToolResponse("call-1", "first"),
				capabilities.NewToolResponse("call-2", "second"),
			},
			wantResults: []prompts.ToolResponse{
				{CallID: "call-1", Content: "first"},
				{CallID: "call-2", Content: "second"},
			},
		},
		{
			name: "skips nil responses",
			responses: []capabilities.ToolResponse{
				nil,
				capabilities.NewToolResponse("call-1", "first"),
			},
			wantResults: []prompts.ToolResponse{{CallID: "call-1", Content: "first"}},
		},
		{
			name:          "empty response list returns nil prompt",
			wantNilPrompt: true,
		},
		{
			name:          "all nil responses return nil prompt",
			responses:     []capabilities.ToolResponse{nil, nil},
			wantNilPrompt: true,
		},
		{
			name: "duplicate call IDs fail",
			responses: []capabilities.ToolResponse{
				capabilities.NewToolResponse("call-1", "first"),
				capabilities.NewToolResponse("call-1", "second"),
			},
			wantError: "duplicate tool call ID",
		},
		{
			name:       "invalid response fails validation",
			responses:  []capabilities.ToolResponse{capabilities.NewToolResponse("", "done")},
			wantFields: []string{"call_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt, err := capabilities.MergeToolResponses(tt.responses...)
			if tt.wantError != "" {
				require.ErrorIs(t, err, capabilities.ErrInvalidToolPrompt)
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			if len(tt.wantFields) > 0 {
				require.ErrorIs(t, err, capabilities.ErrInvalidToolPrompt)
				errors.RequireValidationFields(t, err, tt.wantFields...)
				return
			}

			require.NoError(t, err)
			if tt.wantNilPrompt {
				require.Nil(t, prompt)
				return
			}

			require.Equal(t, prompts.RoleTool, prompt.Role)
			require.Equal(t, prompts.TypeMessage, prompt.Type)
			require.Empty(t, prompt.Content)
			require.Equal(t, tt.wantResults, prompt.ToolResults)
		})
	}
}
