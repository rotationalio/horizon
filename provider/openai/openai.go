// Package openai provides runners for both the Responses and ChatCompletions endpoints
// using the OpenAI API specification. This runner is the most common runner for LLM
// calls and is used both for direct access to OpenAI but also to openrouter, litellm,
// and other providers that support the OpenAI API specification.
package openai

import (
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/version"
)

// Default options for the OpenAI client.
var defaultOptions = []option.RequestOption{
	option.WithHeader("User-Agent", version.UserAgent()), // Set the User-Agent header to the Horizon version.
	option.WithEnvironmentProduction(),                   // Use the production environment by default.
	option.WithMaxRetries(0),                             // Horizon handles retries internally.
}

// New creates an OpenAI client from the provider configuration and resolved provider options.
func New(conf provider.Config, options provider.Options) (*oai.Client, error) {
	opts := make([]option.RequestOption, 0, len(defaultOptions)+6)
	opts = append(opts, defaultOptions...)
	opts = append(opts, option.WithHTTPClient(options.HTTPClient))

	// Add the endpoint to the options. If not set, the client will use the default
	// endpoint, which might come from environment variables (via the sdk).
	if conf.InferenceEndpoint != "" {
		opts = append(opts, option.WithBaseURL(conf.InferenceEndpoint))
	}

	// Add the credentials. APIKey and the older style organization credentials are
	// supported in addition to the token based credentials from an identity provider.
	// TODO: support workload identity provider credentials.
	// See: https://github.com/openai/openai-go#workload-identity-authentication
	if conf.Credentials != nil {
		switch conf.Credentials.Type() {
		case auth.TypeAPIKey:
			apiKey, err := conf.Credentials.APIKey()
			if err != nil {
				return nil, err
			}
			opts = append(opts, option.WithAPIKey(apiKey))
		case auth.TypeOpenAIOrganization:
			apiKey, organization, project, err := conf.Credentials.OpenAIOrganization()
			if err != nil {
				return nil, err
			}
			opts = append(opts,
				option.WithAPIKey(apiKey),
				option.WithOrganization(organization),
				option.WithProject(project),
			)
		case auth.TypeNone:
			// OpenAI-compatible local providers may intentionally require no
			// authentication. An explicit empty key also prevents an ambient
			// OPENAI_API_KEY from being inherited by the SDK.
			opts = append(opts, option.WithAPIKey(""))
		default:
			return nil, errors.New("invalid credential type")
		}
	}

	openAIClient := oai.NewClient(opts...)
	return &openAIClient, nil
}
