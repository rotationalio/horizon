package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/version"
)

// Export selected standard-library functions so callers do not need to import
// net/http just to construct requests through Horizon's client.
// TODO: Add a client-side middleware/transport hook for shared outbound request
// policies, including URL and redirect validation for SSRF prevention.
var (
	NewRequestWithContext = stdhttp.NewRequestWithContext
	DefaultClient         = New()
)

// MaximumRequestTimeout prevents requests without a caller-provided context
// from blocking forever.
const MaximumRequestTimeout = 768 * time.Second

// Returns Horizon's default HTTP client.
func New() *stdhttp.Client {
	return &stdhttp.Client{
		Transport: stdhttp.DefaultTransport,
		Timeout:   MaximumRequestTimeout,
	}
}

// GetOptions controls optional client and response handling for a GET request.
type GetOptions struct {
	Client       *stdhttp.Client
	Accept       string
	MaxBodyBytes int64
}

// Performs an authenticated GET with optional request and response settings.
// Defaults to accepting application/json.
func Get(ctx context.Context, target string, cred auth.RequestCredential, supplied ...GetOptions) ([]byte, error) {
	if len(supplied) > 1 {
		return nil, fmt.Errorf("at most one set of GET options may be supplied")
	}
	options := GetOptions{}
	if len(supplied) == 1 {
		options = supplied[0]
	}
	if options.Accept == "" {
		options.Accept = "application/json"
	}

	if options.MaxBodyBytes < 0 {
		return nil, fmt.Errorf("maximum response body size must not be negative")
	}

	req, err := NewRequestWithContext(ctx, stdhttp.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", options.Accept)
	if cred != nil {
		if err := cred.Set(req); err != nil {
			return nil, err
		}
	}

	client := options.Client
	if client == nil {
		client = DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bodyReader io.Reader = resp.Body
	if options.MaxBodyBytes > 0 {
		readLimit := options.MaxBodyBytes
		if readLimit < int64(^uint64(0)>>1) {
			readLimit++
		}
		bodyReader = io.LimitReader(resp.Body, readLimit)
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}
	if options.MaxBodyBytes > 0 && int64(len(body)) > options.MaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", options.MaxBodyBytes)
	}

	if resp.StatusCode < stdhttp.StatusOK || resp.StatusCode >= stdhttp.StatusMultipleChoices {
		return nil, errors.Join(
			errors.ErrRequestFailed,
			errors.Fmt("%s: %s", resp.Status, strings.TrimSpace(string(body))),
		)
	}
	return body, nil
}

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
