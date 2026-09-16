package provider

import (
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/schema"
)

// An request to an LLM or VLM provider that combines information from both the task
// and the input into a single resource that can be executed by AI.
type Request struct {
	Model        string                  // The name of hte model that the backend will use directly.
	Params       *params.Params          // The parameters to pass to the model
	Input        prompts.Prompts         // The rendered input prompts to pass to the model
	Attachments  attachments.Attachments // Any files, images, links, etc. that are attached to the request
	OutputSchema *schema.Schema          // The schema of the output to return from the LLM (JSON in particular)
	Tools        []string                // Tools advertised to the model that can be used
}

type Response struct{}
