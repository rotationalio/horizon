package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"net/url"

	"go.rtnl.ai/horizon/provider/auth"
)

// Endpoint is an HTTP API endpoint with a reusable base URL, client, and
// request credential.
type Endpoint struct {
	baseURL     *url.URL
	client      *stdhttp.Client
	credentials auth.RequestCredential
}

// Validates and creates an endpoint. A nil client uses the shared default HTTP
// client; nil credentials make unauthenticated requests.
func NewEndpoint(baseURL string, client *stdhttp.Client, credentials auth.RequestCredential) (*Endpoint, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint URL: %w", err)
	}
	if !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("endpoint URL must be an absolute HTTP(S) URL with a host")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("endpoint URL must not contain user information or a fragment")
	}

	copyURL := *parsed
	if client == nil {
		client = DefaultClient
	}
	return &Endpoint{baseURL: &copyURL, client: client, credentials: credentials}, nil
}

// Performs a GET against path, appended to the endpoint base path. A leading
// slash in path is treated as a suffix and does not discard the base.
func (e *Endpoint) Get(ctx context.Context, path string) ([]byte, error) {
	if e == nil || e.baseURL == nil {
		return nil, fmt.Errorf("endpoint is nil")
	}
	target, err := endpointURL(e.baseURL, path)
	if err != nil {
		return nil, err
	}
	return Get(ctx, target, e.credentials, GetOptions{
		Client: e.client,
		Accept: "application/json",
	})
}

// Joins a suffix to the base URL and combines their query parameters.
func endpointURL(base *url.URL, suffix string) (string, error) {
	rel, err := url.Parse(suffix)
	if err != nil {
		return "", err
	}

	out := base.JoinPath(rel.Path)
	query := base.Query()
	for key, values := range rel.Query() {
		query[key] = append(query[key], values...)
	}
	out.RawQuery = query.Encode()
	return out.String(), nil
}
