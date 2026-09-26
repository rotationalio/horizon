package provider

import (
	"fmt"
	"net/url"
	"slices"
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
	if p == nil {
		return errors.ErrUnsupportedProviderType
	}

	if p.APIType == APITypeUnknown {
		return errors.ErrUnsupportedAPIType
	}

	if p.ProviderType == ProviderTypeUnknown {
		return errors.ErrUnsupportedProviderType
	}
	registration, exists := LookupRegistration(p.ProviderType)
	if !exists {
		return errors.Join(errors.ErrUnsupportedProviderType, errors.Fmt("%s is not registered", p.ProviderType))
	}
	if !slices.Contains(registration.APITypes, p.APIType) {
		return errors.Join(errors.ErrUnsupportedAPIType, errors.Fmt("%s does not support %s", p.ProviderType, p.APIType))
	}

	// Mock providers do not make network requests and therefore need no endpoints.
	if p.ProviderType != ProviderTypeMock {
		if p.inference, err = parseEndpoint(p.InferenceEndpoint); err != nil {
			return errors.Join(errors.ErrInvalidInferenceEndpoint, err)
		}
		if p.catalog, err = parseEndpoint(p.CatalogEndpoint); err != nil {
			return errors.Join(errors.ErrInvalidCatalogEndpoint, err)
		}
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
	if !slices.Contains(registration.AuthTypes, p.Credentials.Type()) {
		return errors.Join(errors.ErrInvalidCredentials, errors.Fmt("%s does not support %s credentials", p.ProviderType, p.Credentials.Type()))
	}

	return nil
}

func parseEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("endpoint must use http or https")
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("endpoint must include a host")
	}
	return endpoint, nil
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
	if p == nil || other == nil {
		return p == other
	}
	if p.Credentials == nil || other.Credentials == nil {
		return false
	}
	if p.Validate() != nil || other.Validate() != nil {
		return false
	}

	return p.APIType == other.APIType &&
		p.InferenceEndpoint == other.InferenceEndpoint &&
		p.ProviderType == other.ProviderType &&
		p.CatalogEndpoint == other.CatalogEndpoint &&
		p.DefaultModel == other.DefaultModel &&
		p.Credentials.Type() == other.Credentials.Type()
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
