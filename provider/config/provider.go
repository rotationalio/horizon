package config

import (
	"net/url"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
)

// The configuration for a Horizon provider client.
type Provider struct {
	ID                ulid.ULID          `json:"id" yaml:"id" msg:"id"`                                                          // The ID of this provider configuration
	NoCache           bool               `json:"no_cache" yaml:"no_cache" msg:"no_cache"`                                        // Whether to not cache the provider client (for temporary providers, for example)
	APIType           types.APIType      `json:"api_type" yaml:"api_type" msg:"api_type"`                                        // The API type of the provider
	InferenceEndpoint string             `json:"inference_endpoint" yaml:"inference_endpoint" msg:"inference_endpoint"`          // The inference endpoint URL of the provider
	ProviderType      types.ProviderType `json:"provider_type" yaml:"provider_type" msg:"provider_type"`                         // The provider type of the provider
	CatalogEndpoint   string             `json:"catalog_endpoint" yaml:"catalog_endpoint" msg:"catalog_endpoint"`                // The catalog endpoint URL of the provider
	DefaultModel      string             `json:"default_model" yaml:"default_model" msg:"default_model"`                         // The default model of the provider (used for connectivity checks, for example)
	Credentials       auth.Credential    `json:"credentials,omitempty" yaml:"credentials,omitempty" msg:"credentials,omitempty"` // The optional credentials of the provider

	// Cached URLs

	inference *url.URL
	catalog   *url.URL
}

// Checks that the provider configuration is valid.
func (p *Provider) Validate() error {
	// Require an ID.
	if p.ID.IsZero() {
		return errors.ErrInvalidID
	}

	// Require API type and inference endpoint.
	if p.APIType == types.APITypeUnknown {
		return errors.ErrUnsupportedAPIType
	}
	if _, err := url.Parse(p.InferenceEndpoint); err != nil {
		return errors.Join(errors.ErrInvalidInferenceEndpoint, err)
	}

	// Require provider type and catalog endpoint.
	if p.ProviderType == types.ProviderTypeUnknown {
		return errors.ErrUnsupportedProviderType
	}
	if _, err := url.Parse(p.CatalogEndpoint); err != nil {
		return errors.Join(errors.ErrInvalidCatalogEndpoint, err)
	}

	// Require a default model if the provider type is openai_compatible.
	if p.ProviderType == types.ProviderTypeOpenAICompatible && p.DefaultModel == "" {
		return errors.ErrInvalidDefaultModel
	}

	// Require credentials is not unknown and is valid.
	if p.Credentials == nil || p.Credentials.Type() == auth.TypeUnknown {
		return errors.ErrInvalidCredentials
	}
	if err := p.Credentials.Validate(); err != nil {
		return errors.Join(errors.ErrInvalidCredentials, err)
	}

	return nil
}

// Returns the inference endpoint URL; if the URL is not valid, returns nil.
func (p *Provider) InferenceURL() *url.URL {
	if p.inference == nil {
		p.inference, _ = url.Parse(p.InferenceEndpoint)
	}
	return p.inference
}

// Returns the catalog endpoint URL; if the URL is not valid, returns nil.
func (p *Provider) CatalogURL() *url.URL {
	if p.catalog == nil {
		p.catalog, _ = url.Parse(p.CatalogEndpoint)
	}
	return p.catalog
}

// Compares two provider configurations for equality. Credentials secrets are
// not compared, however their types must match and they must be valid.
func (p *Provider) Equals(other *Provider) bool {
	return p.ID == other.ID &&
		p.APIType == other.APIType &&
		p.InferenceEndpoint == other.InferenceEndpoint &&
		p.ProviderType == other.ProviderType &&
		p.CatalogEndpoint == other.CatalogEndpoint &&
		p.DefaultModel == other.DefaultModel &&
		p.Credentials.Type() == other.Credentials.Type() &&
		p.Credentials.Validate() == nil &&
		other.Validate() == nil
}

// Returns true if the provider configuration is empty.
func (p *Provider) IsZero() bool {
	return p.ID.IsZero() &&
		p.APIType == types.APITypeUnknown &&
		p.InferenceEndpoint == "" &&
		p.ProviderType == types.ProviderTypeUnknown &&
		p.CatalogEndpoint == "" &&
		p.DefaultModel == "" &&
		p.Credentials == nil
}
