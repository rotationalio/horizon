// Package openai provides runners for both the Responses and ChatCompletions endpoints
// using the OpenAI API specification. This runner is the most common runner for LLM
// calls and is used both for direct access to OpenAI but also to openrouter, litellm,
// and other providers that support the OpenAI API specification.
package openai

import (
	"errors"
	"fmt"

	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/version"
)

// Default options for the OpenAI client.
var defaultOptions = []option.RequestOption{
	option.WithHTTPClient(http.DefaultClient),                                         // Use the default Horizon http client from the http package.
	option.WithHeader("User-Agent", fmt.Sprintf("Horizon/%s", version.Version(true))), // Set the User-Agent header to the Horizon version
	option.WithEnvironmentProduction(),                                                // Use the production environment by default
	option.WithMaxRetries(0),                                                          // Horizon handles retries internally
	option.WithRequestTimeout(config.DefaultTimeout),                                  // Set the default request timeout
}

// Creates a new OpenAI client from the Horizon configuration.
// TODO: allow passing in an http client to use for testing.
func New(conf config.Provider) (*oai.Client, error) {
	opts := make([]option.RequestOption, 0, len(defaultOptions)+5)
	opts = append(opts, defaultOptions...)

	// Add the endpoint to the options. If not set, the client will use the default
	// endpoint, which might come from environment variables (via the sdk).
	if conf.InferenceEndpoint != "" {
		opts = append(opts, option.WithBaseURL(conf.InferenceEndpoint))
	}

	// Add the credentials. APIKey and the older style organization credentials are
	// supported in addition to the token based credentials from an identity api.
	// TODO: support workload identity provider credentials.
	// See: https://github.com/openai/openai-go#workload-identity-authentication
	if conf.Credentials != nil {
		switch conf.Credentials.Type() {
		case auth.TypeAPIKey:
			apiKey, err := conf.Credentials.(auth.APIKey).APIKey()
			if err != nil {
				return nil, err
			}
			opts = append(opts, option.WithAPIKey(apiKey))
		case auth.TypeOpenAIOrganization:
			apiKey, organization, project, err := conf.Credentials.(auth.OpenAIOrganization).OpenAIOrganization()
			if err != nil {
				return nil, err
			}
			opts = append(opts,
				option.WithAPIKey(apiKey),
				option.WithOrganization(organization),
				option.WithProject(project),
			)
		default:
			return nil, errors.New("invalid credential type")
		}
	}

	client := oai.NewClient(opts...)
	return &client, nil
}
