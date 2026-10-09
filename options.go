package horizon

import stdhttp "net/http"

// Options are the resolved dependencies and configuration for a Horizon instance.
type Options struct {
	HTTPClient *stdhttp.Client
}

// Option configures a Horizon instance.
type Option func(*Options)

// WithHTTPClient sets the HTTP client shared by Horizon's provider cache and executions.
func WithHTTPClient(client *stdhttp.Client) Option {
	return func(options *Options) {
		if client != nil {
			options.HTTPClient = client
		}
	}
}

// ResolveOptions applies options and returns the resulting Horizon configuration.
func ResolveOptions(options ...Option) Options {
	var resolved Options
	for _, option := range options {
		if option != nil {
			option(&resolved)
		}
	}
	return resolved
}
