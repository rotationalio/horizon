package provider

import (
	"context"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/schema"
)

type Inference interface {
	// Performs an inference [Request] and returns the [Response].
	Generate(ctx context.Context, req *Request) (*Response, error)
}

type Catalog interface {
	// Returns a list of all [catalog.Model] from the provider's catalog.
	FetchCatalog(ctx context.Context) ([]catalog.Model, error)

	// Returns a single [catalog.Model] by the provider's model ID.
	RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error)
}

type Provider interface {
	Inference
	Catalog
}

func New(config Config) Provider {
	//TODO: to implement this, we need to add the openai/openrouter/mock sub-packages
	// from endeavor and adjust them to use Provider interface; OpenRouter's Provider
	// will have its own catalog but for inference it will use the configured OpenAI
	// API implementations; we don't need a cache anymore for this, tasks will have
	// providers, tasks will be on a horizon.Router in memory, and for the "default
	// node provider" we will handle attaching that later on when we finish the runner
	// code.
	return nil
}

// An request to an LLM or VLM provider that combines information from both the task
// and the input into a single resource that can be executed by AI.
type Request struct {
	Model        string                  // The name of the model that the backend will use directly.
	Params       *params.Params          // The parameters to pass to the model
	Input        prompts.Prompts         // The rendered input prompts to pass to the model
	Attachments  attachments.Attachments // Any files, images, links, etc. that are attached to the request
	OutputSchema *schema.Schema          // The schema of the output to return from the LLM (JSON in particular)
	Tools        []string                // Tools advertised to the model that can be used
}

type Response struct {
	// TODO
}
