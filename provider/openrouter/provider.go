package openrouter

import (
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
)

// Implements the Horizon provider interface using OpenRouter's catalog and
// OpenAI-compatible inference APIs.
type Provider struct {
	provider.Generator
	*CatalogClient
}

var _ provider.Provider = (*Provider)(nil)

func init() {
	provider.Register(provider.ProviderTypeOpenRouter, provider.Registration{
		Factory:   NewProvider,
		APITypes:  []provider.APIType{provider.APITypeOpenAIResponses, provider.APITypeOpenAIChatCompletions},
		AuthTypes: []auth.Type{auth.TypeAPIKey},
	})
}

// Constructs an OpenRouter provider.
func NewProvider(conf provider.Config) (p provider.Provider, err error) {
	client := &Provider{}
	switch conf.APIType {
	case provider.APITypeOpenAIResponses:
		if client.Generator, err = openai.NewResponses(conf); err != nil {
			return nil, err
		}
	case provider.APITypeOpenAIChatCompletions:
		if client.Generator, err = openai.NewChatCompletions(conf); err != nil {
			return nil, err
		}
	default:
		return nil, errors.ErrUnsupportedAPIType
	}

	if client.CatalogClient, err = NewCatalog(conf); err != nil {
		return nil, err
	}
	return client, nil
}
