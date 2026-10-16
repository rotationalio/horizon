package capabilities

import (
	"fmt"

	"go.rtnl.ai/horizon/prompts"
)

// ResponseKind identifies the contribution returned after a capability executes.
type ResponseKind uint8

const (
	ResoponseKindUnknown ResponseKind = iota

	// Wants a tool message with text-only content. Zero is reserved for invalid responses.
	ResponseKindToolTextContent
)

// Response is a concrete, tagged contribution from a capability executor.
type Response struct {
	Kind    ResponseKind
	Content string
	// TODO: support other types of content than text-only
}

// NewToolTextContentResponse creates a tool role message with text content for a
// model-requested tool call.
func NewToolTextContentResponse(content string) Response {
	return Response{Kind: ResponseKindToolTextContent, Content: content}
}

// Process converts a contribution into conversation messages correlated to the
// originating call. The caller supplies a validated call ID; the executor does
// not control correlation. Tool text content always produces exactly one message
// containing exactly one tool result. Unsupported kinds return no messages.
func (r Response) Process(callID string) (prompts.Prompts, error) {
	switch r.Kind {
	case ResponseKindToolTextContent:
		return prompts.Prompts{{
			Role: prompts.RoleTool,
			Type: prompts.TypeMessage,
			ToolResults: []prompts.ToolResponse{{
				CallID:  callID,
				Content: r.Content,
			}},
		}}, nil
	default:
		return nil, fmt.Errorf("%w: unknown response kind %d", ErrUnsupportedResponse, r.Kind)
	}
}

// Validate rejects response kinds that Horizon does not yet know how to process.
func (r Response) Validate() error {
	if r.Kind != ResponseKindToolTextContent {
		return fmt.Errorf("%w: unknown response kind %d", ErrUnsupportedResponse, r.Kind)
	}
	return nil
}
