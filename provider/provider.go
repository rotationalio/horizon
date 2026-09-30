// Package provider defines provider-neutral configuration, requests, responses,
// catalog access, and provider construction.
//
// Most programs should use horizon.NewProvider, which registers all built-in
// implementations automatically:
//
//	import (
//		"context"
//		"os"
//
//		"go.rtnl.ai/horizon"
//		"go.rtnl.ai/horizon/prompts"
//		"go.rtnl.ai/horizon/provider"
//		"go.rtnl.ai/horizon/provider/auth"
//	)
//
//	client, err := horizon.NewProvider(provider.Config{
//		APIType:           provider.APITypeOpenAIResponses,
//		ProviderType:      provider.ProviderTypeOpenAI,
//		InferenceEndpoint: "https://api.openai.com/v1",
//		CatalogEndpoint:   "https://api.openai.com/v1/models",
//		DefaultModel:      "gpt-5-mini",
//		Credentials:       auth.NewAPIKey(os.Getenv("OPENAI_API_KEY")),
//	})
//	if err != nil {
//		// Handle invalid configuration or construction failure.
//	}
//
//	response, err := client.Generate(context.Background(), &provider.Request{
//		Model: "gpt-5-mini",
//		Input: prompts.Prompts{
//			{Role: prompts.RoleUser, Content: "What is the capital of France?"},
//		},
//	})
//	if err != nil {
//		// Handle the provider request failure.
//	}
//	_ = response
//
// Programs using this low-level package's [New] function directly must import
// the desired implementation package, directly or for side effects:
//
//	import (
//		"go.rtnl.ai/horizon/provider"
//		_ "go.rtnl.ai/horizon/provider/openai"
//	)
//
//	client, err := provider.New(config)
//
// Importing an implementation directly is preferable when its concrete API is
// needed in addition to the provider-neutral interface.
package provider

import (
	"context"

	"go.rtnl.ai/horizon/provider/catalog"
)

// Generates provider-neutral inference responses.
type Generator interface {
	// Performs an inference [Request] and returns the [Response].
	Generate(ctx context.Context, req *Request) (*Response, error)
}

// Performs inference and exposes the model catalog for one configured external
// provider.
type Provider interface {
	Generator

	// Returns a list of all [catalog.Model] from the provider's catalog.
	FetchCatalog(ctx context.Context) ([]catalog.Model, error)

	// Returns a single [catalog.Model] by the provider's model ID.
	RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error)
}
