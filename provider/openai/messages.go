package openai

import "go.rtnl.ai/horizon/prompts"

// Associates request-level attachments with the last user message. Direct adapter
// callers can also submit attachment-only input, without constructing a prompt.
func attachmentMessages(in prompts.Prompts, hasAttachments bool) (prompts.Prompts, int) {
	if !hasAttachments {
		return in, -1
	}

	// Return the last user role message index.
	for i := len(in) - 1; i >= 0; i-- {
		if in[i] != nil && in[i].Role == prompts.RoleUser {
			return in, i
		}
	}

	// Add an attachment-only message to the end of the prompt list.
	in = append(in, &prompts.Prompt{
		Role:    prompts.RoleUser,
		Type:    prompts.TypeMessage,
		Content: "This message contains only attachments.",
	})
	return in, len(in) - 1
}
