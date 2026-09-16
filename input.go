package horizon

import (
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
)

// Input is the raw input data that gets passed to a task to execute it. The input
// can come from a user and should supply the context information needed to render a
// prompt as well as any input attachments such as image references, files, videos,
// etc. that are returned from the task execution.
type Input struct {
	// The source of the input (can be a user, but can also be previous tasks)
	Source string `json:"source,omitzero"`

	// The unique ID of the previous response to the model. Use this to
	// create multi-turn conversations
	PreviousResponseID string `json:"previous_response_id,omitzero"`

	// A system (or developer) message inserted into the model's context. When used
	// along with `previous_response_id`, the instructions from a previous response
	// will not be carried over to the next response. This makes it simple to swap out
	// system (or developer) messages in new responses.
	Instructions string `json:"instructions,omitzero"`

	// The context of the input that is used to render the prompt template.
	Context prompts.Context `json:"context,omitzero"`

	// Any attachments from a multipart request that are attached to the input.
	Attachments attachments.Attachments `json:"attachments,omitzero"`
}
