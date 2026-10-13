package capabilities_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/prompts"
)

// The text-response constructor preserves the supplied content and assigns the
// supported text-result kind, so the returned response passes validation.
func TestNewToolTextContentResponse(t *testing.T) {
	response := capabilities.NewToolTextContentResponse("done")

	require.Equal(t, capabilities.Response{
		Kind:    capabilities.ResponseKindToolTextContent,
		Content: "done",
	}, response)
	require.NoError(t, response.Validate())
}

// Processing a supported response creates a tool message correlated to the
// originating call. Unknown kinds must not contribute any messages, even when
// they contain text that would otherwise be usable as a result.
func TestResponseProcess(t *testing.T) {
	// Arrange: explicitly constructed text results may be empty. A zero-value
	// response or a kind without a processing path must be rejected.
	tests := []struct {
		name     string
		response capabilities.Response
		valid    bool
	}{
		{
			name:     "text result",
			response: capabilities.NewToolTextContentResponse("done"),
			valid:    true,
		},
		{
			name:     "empty text result",
			response: capabilities.NewToolTextContentResponse(""),
			valid:    true,
		},
		{
			name:     "zero value is invalid",
			response: capabilities.Response{},
		},
		{
			name:     "unknown kind",
			response: capabilities.Response{Kind: capabilities.ResponseKind(255), Content: "unsafe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act: Horizon supplies the call ID independently of the executor's response.
			messages, err := tt.response.Process("call-1")

			// Assert: failures produce no partial messages; supported results produce
			// exactly one correlated tool result without changing the response.
			if !tt.valid {
				require.ErrorIs(t, err, capabilities.ErrUnsupportedResponse)
				require.Nil(t, messages)
				return
			}
			require.NoError(t, err)
			require.Equal(t, prompts.Prompts{{
				Role:        prompts.RoleTool,
				Type:        prompts.TypeMessage,
				ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: tt.response.Content}},
			}}, messages)

			// Each conversion owns its message containers and uses its supplied ID.
			messages[0].ToolResults[0].Content = "changed"
			again, err := tt.response.Process("call-2")
			require.NoError(t, err)
			require.Equal(t, tt.response.Content, again[0].ToolResults[0].Content)
			require.Equal(t, "call-2", again[0].ToolResults[0].CallID)
		})
	}
}

// Only response kinds with an explicit Horizon processing path are valid.
func TestResponseValidate(t *testing.T) {
	tests := []struct {
		name     string
		response capabilities.Response
		valid    bool
	}{
		{
			name:     "tool result",
			response: capabilities.NewToolTextContentResponse("done"),
			valid:    true,
		},
		{
			name:     "empty text result",
			response: capabilities.NewToolTextContentResponse(""),
			valid:    true,
		},
		{
			name:     "zero value is invalid",
			response: capabilities.Response{},
		},
		{
			name: "unknown kind",
			response: capabilities.Response{
				Kind: capabilities.ResponseKind(255),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, capabilities.ErrUnsupportedResponse)
		})
	}
}
