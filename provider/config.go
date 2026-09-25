package provider

import (
	"net/url"
	"time"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
)

const DefaultRequestTimeout = 128 * time.Second

// The configuration for building a Horizon [Provider].
type Config struct {
	// The API type that this provider uses (ex: "chat_completions", "requests")
	APIType APIType `json:"api_type" yaml:"api_type" msg:"api_type"`
	// The base endpoint URL for the provider's inference API.
	InferenceEndpoint string `json:"inference_endpoint" yaml:"inference_endpoint" msg:"inference_endpoint"`
	// The provider's type (ex: "openai", "openrouter", etc.)
	ProviderType ProviderType `json:"provider_type" yaml:"provider_type" msg:"provider_type"`
	// The base endpoint URL for the provider's model catalog API.
	CatalogEndpoint string `json:"catalog_endpoint" yaml:"catalog_endpoint" msg:"catalog_endpoint"`
	// The default model to use for the provider. This is used for system tasks such
	// as connectivity checks and catalog refreshing.
	DefaultModel string `json:"default_model" yaml:"default_model" msg:"default_model"`
	// The credentials to use when connecting to this provider.
	Credentials auth.Credential `json:"credentials,omitempty" yaml:"credentials,omitempty" msg:"credentials,omitempty"`

	inference *url.URL
	catalog   *url.URL
}

func (p *Config) Validate() (err error) {
	// Require API type and inference endpoint. Also cache it now to save a parsing.
	if p.APIType == APITypeUnknown {
		return errors.ErrUnsupportedAPIType
	}
	if p.inference, err = url.Parse(p.InferenceEndpoint); err != nil {
		return errors.Join(errors.ErrInvalidInferenceEndpoint, err)
	}

	// Require provider type and catalog endpoint. Also cache it now to save a parsing.
	if p.ProviderType == ProviderTypeUnknown {
		return errors.ErrUnsupportedProviderType
	}
	if p.catalog, err = url.Parse(p.CatalogEndpoint); err != nil {
		return errors.Join(errors.ErrInvalidCatalogEndpoint, err)
	}

	// Require a default model if the provider type is openai_compatible. Other
	// provider types have preferred default models in their packages, so this
	// is optional for them.
	if p.ProviderType == ProviderTypeOpenAICompatible && p.DefaultModel == "" {
		return errors.ErrInvalidDefaultModel
	}

	// Require credentials is not unknown and is valid. Use can still provide
	// the [auth.TypeNone] credential for "no auth".
	if p.Credentials == nil || p.Credentials.Type() == auth.TypeUnknown {
		return errors.ErrInvalidCredentials
	}
	if err = p.Credentials.Validate(); err != nil {
		return errors.Join(errors.ErrInvalidCredentials, err)
	}

	return nil
}

// Returns the inference endpoint URL. If the URL in the config is invalid,
// returns nil; call [Config.Validate] to validate this URL parses correctly.
func (p *Config) InferenceURL() *url.URL {
	if p.inference == nil {
		p.inference, _ = url.Parse(p.InferenceEndpoint)
	}
	return p.inference
}

// Returns the catalog endpoint URL. If the URL in the config is invalid,
// returns nil; call [Config.Validate] to validate this URL parses correctly.
func (p *Config) CatalogURL() *url.URL {
	if p.catalog == nil {
		p.catalog, _ = url.Parse(p.CatalogEndpoint)
	}
	return p.catalog
}

// Compares two provider configurations for equality. Credentials secrets are
// not compared, however their types must match and they must both be valid.
func (p *Config) Equals(other *Config) bool {
	return p.APIType == other.APIType &&
		p.InferenceEndpoint == other.InferenceEndpoint &&
		p.ProviderType == other.ProviderType &&
		p.CatalogEndpoint == other.CatalogEndpoint &&
		p.DefaultModel == other.DefaultModel &&
		p.Credentials.Type() == other.Credentials.Type() &&
		p.Credentials.Validate() == nil &&
		other.Validate() == nil
}

// Returns true if the provider configuration is empty.
func (p *Config) IsZero() bool {
	return p.APIType == APITypeUnknown &&
		p.InferenceEndpoint == "" &&
		p.ProviderType == ProviderTypeUnknown &&
		p.CatalogEndpoint == "" &&
		p.DefaultModel == "" &&
		p.Credentials == nil
}
