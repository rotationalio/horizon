package provider

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/validation"
)

// The configuration for building a Horizon [Provider].
type Config struct {
	// The application-assigned ID used to identify this provider.
	ID ulid.ULID `json:"id" yaml:"id" msg:"id"`
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
	Credentials *auth.Credentials `json:"credentials,omitempty" yaml:"credentials,omitempty" msg:"credentials,omitempty"`

	inference *url.URL
	catalog   *url.URL
}

// Hash fingerprints all configuration fields, including credentials.
// Hash values must not be logged or exposed alongside credential data.
func (p Config) Hash() ([sha256.Size]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func (p *Config) Validate() (err error) {
	if p == nil {
		return validation.Error(nil, validation.Missing("provider"))
	}
	var causes []error
	if p.ID == (ulid.ULID{}) {
		err = validation.Error(err, validation.Missing("id"))

	}

	registration, exists := LookupRegistration(p.ProviderType)
	if p.ProviderType == ProviderTypeUnknown {
		err = validation.Error(err, validation.Missing("provider_type"))
	} else if !exists {
		err = validation.Error(err, validation.Incorrect("provider_type", "is not registered"))
	}
	if p.APIType == APITypeUnknown {
		err = validation.Error(err, validation.Missing("api_type"))
	} else if exists && !slices.Contains(registration.APITypes, p.APIType) {
		err = validation.Error(err, validation.Incorrect("api_type", fmt.Sprintf("%s does not support %s", p.ProviderType, p.APIType)))
	}

	// Mock providers do not make network requests and therefore need no endpoints.
	if p.ProviderType != ProviderTypeMock {
		var endpointErr error
		if p.inference, endpointErr = parseEndpoint(p.InferenceEndpoint); endpointErr != nil {
			err = validation.Error(err, validation.Incorrect("inference_endpoint", endpointErr.Error()))
			causes = append(causes, endpointErr)
		}
		if p.catalog, endpointErr = parseEndpoint(p.CatalogEndpoint); endpointErr != nil {
			err = validation.Error(err, validation.Incorrect("catalog_endpoint", endpointErr.Error()))
			causes = append(causes, endpointErr)
		}
	}

	// Require a default model if the provider type is openai_compatible. Other
	// provider types have preferred default models in their packages, so this
	// is optional for them.
	if p.ProviderType == ProviderTypeOpenAICompatible && p.DefaultModel == "" {
		err = validation.Error(err, validation.Missing("default_model"))

	}

	// Require credentials is not unknown and is valid. Users can still provide
	// the [auth.TypeNone] credential for "no auth".
	if p.Credentials == nil || p.Credentials.Type() == auth.TypeUnknown {
		err = validation.Error(err, validation.Missing("credentials"))
	} else if credentialErr := p.Credentials.Validate(); credentialErr != nil {
		err = validation.SubfieldError(err, credentialErr, "credentials")
		causes = append(causes, credentialErr)
	} else if exists && !slices.Contains(registration.AuthTypes, p.Credentials.Type()) {
		err = validation.Error(err, validation.Incorrect("credentials", fmt.Sprintf("%s does not support %s credentials", p.ProviderType, p.Credentials.Type())))
	}

	return errors.Join(err, errors.Join(causes...))
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

	return p.ID == other.ID &&
		p.APIType == other.APIType &&
		p.InferenceEndpoint == other.InferenceEndpoint &&
		p.ProviderType == other.ProviderType &&
		p.CatalogEndpoint == other.CatalogEndpoint &&
		p.DefaultModel == other.DefaultModel &&
		p.Credentials.Type() == other.Credentials.Type()
}

// Returns true if the provider configuration is empty.
func (p *Config) IsZero() bool {
	return p.ID.IsZero() &&
		p.APIType == APITypeUnknown &&
		p.InferenceEndpoint == "" &&
		p.ProviderType == ProviderTypeUnknown &&
		p.CatalogEndpoint == "" &&
		p.DefaultModel == "" &&
		p.Credentials == nil
}
