package provider_test

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/validation"
	"go.yaml.in/yaml/v3"
	"golang.org/x/oauth2"
)

// Verifies provider configuration validation accepts supported combinations and
// reports invalid fields while retaining classification and diagnostic causes.
func TestProviderValidate(t *testing.T) {
	t.Run("accepts valid config", func(t *testing.T) {
		require.NoError(t, validProvider().Validate())
	})

	tests := []struct {
		name   string
		change func(*provider.Config)
		field  string
	}{
		{
			name: "requires provider ID",
			change: func(c *provider.Config) {
				c.ID = ulid.ULID{}
			},
			field: "id",
		},
		{
			name: "requires supported API type",
			change: func(c *provider.Config) {
				c.APIType = provider.APITypeUnknown
			},
			field: "api_type",
		},
		{
			name: "requires valid inference endpoint",
			change: func(c *provider.Config) {
				c.InferenceEndpoint = "://bad"
			},
			field: "inference_endpoint",
		},
		{
			name: "requires absolute inference endpoint",
			change: func(c *provider.Config) {
				c.InferenceEndpoint = "/v1"
			},
			field: "inference_endpoint",
		},
		{
			name: "requires HTTP inference endpoint",
			change: func(c *provider.Config) {
				c.InferenceEndpoint = "ftp://example.com/v1"
			},
			field: "inference_endpoint",
		},
		{
			name: "requires supported provider type",
			change: func(c *provider.Config) {
				c.ProviderType = provider.ProviderTypeUnknown
			},
			field: "provider_type",
		},
		{
			name: "requires registered provider type",
			change: func(c *provider.Config) {
				c.ProviderType = provider.ProviderType(255)
			},
			field: "provider_type",
		},
		{
			name: "requires valid catalog endpoint",
			change: func(c *provider.Config) {
				c.CatalogEndpoint = "://bad"
			},
			field: "catalog_endpoint",
		},
		{
			name: "requires absolute catalog endpoint",
			change: func(c *provider.Config) {
				c.CatalogEndpoint = "/models"
			},
			field: "catalog_endpoint",
		},
		{
			name: "requires provider API support",
			change: func(c *provider.Config) {
				c.APIType = provider.APITypeMock
			},
			field: "api_type",
		},
		{
			name: "requires credentials",
			change: func(c *provider.Config) {
				c.Credentials = nil
			},
			field: "credentials",
		},
		{
			name: "requires valid credentials",
			change: func(c *provider.Config) {
				c.Credentials = auth.NewAPIKey("")
			},
			field: "credentials.api_key",
		},
		{
			name: "requires provider credential support",
			change: func(c *provider.Config) {
				c.Credentials = auth.NewBasic("username", "password")
			},
			field: "credentials",
		},
		{
			name: "requires valid default model",
			change: func(c *provider.Config) {
				c.ProviderType = provider.ProviderTypeOpenAICompatible
				c.DefaultModel = ""
			},
			field: "default_model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := validProvider()
			tt.change(conf)
			err := conf.Validate()
			validation.RequireValidationFields(t, err, tt.field)
		})
	}

	t.Run("nil config", func(t *testing.T) {
		var conf *provider.Config
		err := conf.Validate()
		validation.RequireValidation(t, err, validation.Missing("provider"))
	})

	t.Run("reports independent failures together", func(t *testing.T) {
		conf := validProvider()
		conf.ID = ulid.ULID{}
		conf.InferenceEndpoint = "/v1"
		conf.CatalogEndpoint = "/models"
		conf.Credentials = nil
		err := conf.Validate()
		validation.RequireValidationFields(t, err, "id", "inference_endpoint", "catalog_endpoint", "credentials")
	})

	t.Run("preserves endpoint parse cause", func(t *testing.T) {
		conf := validProvider()
		conf.InferenceEndpoint = "://bad"
		err := conf.Validate()
		var parseErr *url.Error
		require.ErrorAs(t, err, &parseErr)
		validation.RequireValidationFields(t, err, "inference_endpoint")
	})

	t.Run("reports credential subfields", func(t *testing.T) {
		conf := validProvider()
		conf.Credentials = auth.NewOpenAIOrganization("", "", "")
		err := conf.Validate()
		validation.RequireValidationFields(t, err,
			"credentials.api_key",
			"credentials.organization",
			"credentials.project",
		)
	})

	t.Run("mock requires no endpoints", func(t *testing.T) {
		conf := &provider.Config{
			ID:           providerTestID,
			APIType:      provider.APITypeMock,
			ProviderType: provider.ProviderTypeMock,
			Credentials:  auth.NewNone(),
		}
		require.NoError(t, conf.Validate())
	})

	t.Run("OpenAI compatible supports no authentication", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = provider.ProviderTypeOpenAICompatible
		conf.Credentials = auth.NewNone()
		require.NoError(t, conf.Validate())
	})
}

// Verifies configuration hashes include identity, settings, and credential values.
func TestProviderHash(t *testing.T) {
	t.Run("stable across validation", func(t *testing.T) {
		config := validProvider()
		before, err := config.Hash()
		require.NoError(t, err)
		require.NoError(t, config.Validate())
		after, err := config.Hash()
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	tests := []struct {
		name   string
		change func(*provider.Config)
	}{
		{
			name:   "ID",
			change: func(c *provider.Config) { c.ID = ulid.Make() },
		},
		{
			name:   "API type",
			change: func(c *provider.Config) { c.APIType = provider.APITypeOpenAIResponses },
		},
		{
			name:   "provider type",
			change: func(c *provider.Config) { c.ProviderType = provider.ProviderTypeOpenAI },
		},
		{
			name:   "inference endpoint",
			change: func(c *provider.Config) { c.InferenceEndpoint = "https://other.example/v1" },
		},
		{
			name:   "catalog endpoint",
			change: func(c *provider.Config) { c.CatalogEndpoint = "https://other.example/models" },
		},
		{
			name:   "default model",
			change: func(c *provider.Config) { c.DefaultModel = "other-model" },
		},
		{
			name:   "credentials",
			change: func(c *provider.Config) { c.Credentials = auth.NewAPIKey("rotated-secret") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validProvider()
			before, err := config.Hash()
			require.NoError(t, err)
			tt.change(config)
			after, err := config.Hash()
			require.NoError(t, err)
			require.NotEqual(t, before, after)
		})
	}

	t.Run("preserves marshal cause", func(t *testing.T) {
		config := validProvider()
		config.Credentials = auth.NewOAuth2Token(&oauth2.Token{
			AccessToken: "secret",
			Expiry:      time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
		})
		_, err := config.Hash()
		var marshalErr *json.MarshalerError
		require.ErrorAs(t, err, &marshalErr)
		require.Contains(t, marshalErr.Err.Error(), "year outside of range")
	})
}

// Verifies concrete credentials round-trip through configuration files without
// custom config decoders, including absent credentials and each built-in type.
func TestProviderConfigSerialization(t *testing.T) {
	tests := []struct {
		name        string
		credentials *auth.Credentials
	}{
		{
			name: "absent",
		},
		{
			name:        "none",
			credentials: auth.NewNone(),
		},
		{
			name:        "API key",
			credentials: auth.NewAPIKey("secret"),
		},
		{
			name:        "token",
			credentials: auth.NewToken("secret"),
		},
		{
			name:        "basic",
			credentials: auth.NewBasic("user", "password"),
		},
		{
			name:        "OAuth2 client",
			credentials: auth.NewOAuth2Client("client", "secret"),
		},
		{
			name: "OAuth2 token",
			credentials: auth.NewOAuth2Token(&oauth2.Token{
				AccessToken:  "access",
				RefreshToken: "refresh",
				TokenType:    "Bearer",
				Expiry:       time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC),
			}),
		},
		{
			name:        "OpenAI organization",
			credentials: auth.NewOpenAIOrganization("key", "org", "project"),
		},
	}
	for _, format := range []string{"JSON", "YAML"} {
		t.Run(format, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					original := validProvider()
					original.Credentials = tt.credentials
					var decoded provider.Config
					if format == "JSON" {
						data, err := json.Marshal(original)
						require.NoError(t, err)
						require.NoError(t, json.Unmarshal(data, &decoded))
					} else {
						data, err := yaml.Marshal(original)
						require.NoError(t, err)
						require.NoError(t, yaml.Unmarshal(data, &decoded))
					}
					require.Equal(t, *original, decoded)
					before, err := original.Hash()
					require.NoError(t, err)
					after, err := decoded.Hash()
					require.NoError(t, err)
					require.Equal(t, before, after)
				})
			}
		})
	}
}

// Verifies provider configuration equality is nil-safe, ignores credential
// secrets, and rejects semantically different configurations.
func TestProviderEquals(t *testing.T) {
	var nilConfig *provider.Config
	require.True(t, nilConfig.Equals(nil))
	require.False(t, nilConfig.Equals(validProvider()))
	require.False(t, validProvider().Equals(nil))

	left := validProvider()
	right := validProvider()
	right.Credentials = auth.NewAPIKey("different-secret")
	require.True(t, left.Equals(right), "credential values are intentionally excluded")

	right.APIType = provider.APITypeMock
	require.False(t, left.Equals(right))
}

var providerTestID = ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

func validProvider() *provider.Config {
	return &provider.Config{
		ID:                providerTestID,
		InferenceEndpoint: "https://openrouter.ai/api/v1",
		CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
		ProviderType:      provider.ProviderTypeOpenRouter,
		APIType:           provider.APITypeOpenAIChatCompletions,
		DefaultModel:      "default/model",
		Credentials:       auth.NewAPIKey("secret"),
	}
}
