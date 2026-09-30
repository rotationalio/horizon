package openai

import (
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
)

// Implements the Horizon provider interface using OpenAI APIs.
type Provider struct {
	provider.Generator
	*CatalogClient
}

var _ provider.Provider = (*Provider)(nil)

func init() {
	provider.Register(provider.ProviderTypeOpenAI, provider.Registration{
		Factory:   NewProvider,
		APITypes:  []provider.APIType{provider.APITypeOpenAIResponses, provider.APITypeOpenAIChatCompletions},
		AuthTypes: []auth.Type{auth.TypeAPIKey, auth.TypeOpenAIOrganization},
	})
	provider.Register(provider.ProviderTypeOpenAICompatible, provider.Registration{
		Factory: NewProvider,
		// OpenAI compatible has CC as the first (should be treated as primary/default)
		// because most local providers support CC. Responses is the preferred API
		// overall in most cases, however.
		APITypes: []provider.APIType{provider.APITypeOpenAIChatCompletions, provider.APITypeOpenAIResponses},
		AuthTypes: []auth.Type{
			auth.TypeAPIKey,
			auth.TypeOpenAIOrganization,
			auth.TypeNone, // For local LLMs or free providers
		},
	})
}

// Constructs an OpenAI or OpenAI-compatible provider.
func NewProvider(conf provider.Config) (p provider.Provider, err error) {
	client := &Provider{}
	switch conf.APIType {
	case provider.APITypeOpenAIResponses:
		if client.Generator, err = NewResponses(conf); err != nil {
			return nil, err
		}
	case provider.APITypeOpenAIChatCompletions:
		if client.Generator, err = NewChatCompletions(conf); err != nil {
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
