package provider

import (
	stdhttp "net/http"

	"go.rtnl.ai/horizon/http"
)

// Options are the resolved dependencies for a provider implementation.
type Options struct {
	HTTPClient *stdhttp.Client
}

// Option configures provider construction.
type Option func(*Options)

// WithHTTPClient sets the HTTP client used by constructed providers.
func WithHTTPClient(client *stdhttp.Client) Option {
	return func(options *Options) {
		if client != nil {
			options.HTTPClient = client
		}
	}
}

// ResolveOptions applies options and returns the resulting provider dependencies.
func ResolveOptions(options ...Option) Options {
	resolved := Options{HTTPClient: http.DefaultClient}
	for _, option := range options {
		if option != nil {
			option(&resolved)
		}
	}
	return resolved
}
