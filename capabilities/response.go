package capabilities

import (
	"fmt"
	"slices"

	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/x/validation"
)

// Response is the base result contract for an executed capability.
type Response interface {
	// Returns the original call ID from the request.
	CallID() string
	// Validates the response.
	Validate() error
}

// ToolResponse contributes a tool role message for a correlated
// capability call.
type ToolResponse interface {
	Response
	// Returns a tool role prompt with the response text and call correlation.
	ToolPrompt() *prompts.Prompt
}

// Constructs a new ToolResponse with the given call ID and text content.
// TODO: support more than just textual string content here, such as files and binary data.
func NewToolResponse(callID, text string) ToolResponse {
	return toolResponse{
		callID: callID,
		prompt: &prompts.Prompt{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  callID,
				Content: text,
			}},
		},
	}
}

// Implements [ToolResponse].
type toolResponse struct {
	callID string
	prompt *prompts.Prompt
}

// Returns the original call ID from the request.
func (r toolResponse) CallID() string { return r.callID }

// Validates the response.
func (r toolResponse) Validate() error {
	var fieldErrors []*validation.FieldError
	if r.callID == "" {
		fieldErrors = append(fieldErrors, validation.Missing("call_id"))
	}

	p := r.prompt
	if p == nil {
		fieldErrors = append(fieldErrors, validation.Missing("tool_prompt"))
	} else {
		if p.Role != prompts.RoleTool {
			fieldErrors = append(fieldErrors, validation.Incorrect("role", fmt.Sprintf("expected tool, got %s", p.Role)))
		}
		if p.Type != prompts.TypeMessage {
			fieldErrors = append(fieldErrors, validation.Incorrect("type", fmt.Sprintf("expected message, got %s", p.Type)))
		}
		if len(p.ToolResults) != 1 {
			fieldErrors = append(fieldErrors, validation.Incorrect("tool_results", "must contain exactly one result"))
		} else if p.ToolResults[0].CallID != r.callID {
			fieldErrors = append(fieldErrors, validation.Incorrect("tool_results.call_id", "must match call_id"))
		}
		if p.Content != "" {
			fieldErrors = append(fieldErrors, validation.Incorrect("content", "must be empty when tool results are present"))
		}
	}

	if len(fieldErrors) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrInvalidToolPrompt, validation.Error(nil, fieldErrors...))
}

// Returns a tool role prompt with the response text.
func (r toolResponse) ToolPrompt() *prompts.Prompt {
	return r.prompt
}

// MergeToolResponses combines multiple tool responses into a single prompt. Nil
// responses are skipped. Returns nil if no valid responses are provided.
func MergeToolResponses(responses ...ToolResponse) (*prompts.Prompt, error) {
	responses = slices.DeleteFunc(responses, func(r ToolResponse) bool {
		return r == nil
	})
	if len(responses) == 0 {
		return nil, nil
	}

	toolResults := make([]prompts.ToolResponse, 0, len(responses))
	seen := make(map[string]struct{}, len(responses))
	for _, r := range responses {
		if err := r.Validate(); err != nil {
			return nil, err
		}
		for _, result := range r.ToolPrompt().ToolResults {
			if _, ok := seen[result.CallID]; ok {
				return nil, fmt.Errorf("%w: duplicate tool call ID %q", ErrInvalidToolPrompt, result.CallID)
			}
			seen[result.CallID] = struct{}{}
			toolResults = append(toolResults, result)
		}
	}

	return &prompts.Prompt{
		Role:        prompts.RoleTool,
		Type:        prompts.TypeMessage,
		ToolResults: toolResults,
	}, nil
}

// For the future, these are some other types we might want to use:
// TODO: UserMessageResponse: adds a user role message (useful for MCP prompts and for adding general text-based message context)
// TODO: InputAttachmentResponse: adds an Attachment (a file) to the input for the next round of inference
// TODO: OutputAttachmentResponse: adds an Attachment (a file) to the output for the user
