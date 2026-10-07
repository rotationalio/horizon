package openrouter

import (
	stdhttp "net/http"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/ulid"
)

// Implements the Horizon provider interface using OpenRouter's catalog and
// OpenAI-compatible inference APIs.
type Provider struct {
	id ulid.ULID
	provider.Generator
	*CatalogClient
}

var _ provider.Provider = (*Provider)(nil)

// ID returns the provider configuration ID.
func (p *Provider) ID() ulid.ULID {
	return p.id
}

func init() {
	provider.Register(provider.ProviderTypeOpenRouter, provider.Registration{
		Factory:   NewProviderWithHTTPClient,
		APITypes:  []provider.APIType{provider.APITypeOpenAIResponses, provider.APITypeOpenAIChatCompletions},
		AuthTypes: []auth.Type{auth.TypeAPIKey},
	})
}

// NewProvider constructs an OpenRouter provider using Horizon's default HTTP client.
func NewProvider(conf provider.Config) (provider.Provider, error) {
	return NewProviderWithHTTPClient(conf, nil)
}

// NewProviderWithHTTPClient constructs an OpenRouter provider using client.
func NewProviderWithHTTPClient(conf provider.Config, httpClient *stdhttp.Client) (p provider.Provider, err error) {
	client := &Provider{id: conf.ID}
	switch conf.APIType {
	case provider.APITypeOpenAIResponses:
		if client.Generator, err = openai.NewResponsesWithHTTPClient(conf, httpClient); err != nil {
			return nil, err
		}
	case provider.APITypeOpenAIChatCompletions:
		if client.Generator, err = openai.NewChatCompletionsWithHTTPClient(conf, httpClient); err != nil {
			return nil, err
		}
	default:
		return nil, errors.ErrUnsupportedAPIType
	}

	if client.CatalogClient, err = NewCatalogWithHTTPClient(conf, httpClient); err != nil {
		return nil, err
	}
	return client, nil
}
