package capabilities

import "go.rtnl.ai/horizon/prompts"

// NewTestToolResponse exposes a tool response with arbitrary prompt data to external tests.
func NewTestToolResponse(callID string, prompt *prompts.Prompt) ToolResponse {
	return toolResponse{callID: callID, prompt: prompt}
}
