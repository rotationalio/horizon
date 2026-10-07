package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"

	"go.rtnl.ai/horizon/config"
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

// A JSON object is a map of string keys to any values.
type JSON map[string]any

// New returns a fresh HTTP client using the configured HTTP timeout.
func New() *stdhttp.Client {
	conf, err := config.Get()
	client := &stdhttp.Client{Transport: stdhttp.DefaultTransport}
	if err == nil {
		client.Timeout = conf.HTTPClientTimeout
	}
	return client
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
