package http

import (
	"bytes"
	"context"
	"encoding/json"
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
//
// TODO: Add a client-side middleware/transport hook for shared outbound request
// policies, including URL and redirect validation for SSRF prevention.
var (
	NewRequestWithContext = stdhttp.NewRequestWithContext
	DefaultClient         *stdhttp.Client
)

func init() {
	var err error
	if DefaultClient, err = New(); err != nil {
		panic(fmt.Errorf("could not load Horizon configuration: %w", err))
	}
}

// A JSON object is a map of string keys to any values.
type JSON map[string]any

// New returns a fresh HTTP client using the configured HTTP timeout.
func New() (*stdhttp.Client, error) {
	conf, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load HTTP client configuration: %w", err)
	}
	return &stdhttp.Client{Transport: stdhttp.DefaultTransport, Timeout: conf.HTTPClientTimeout}, nil
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

	req, err := newRequest(ctx, stdhttp.MethodGet, target, nil, options.Accept, cred)
	if err != nil {
		return nil, err
	}
	return send(req, options.Client, options.MaxBodyBytes)
}

// Performs an authenticated POST with body encoded as JSON. The response is
// expected to be JSON. A nil client uses [DefaultClient].
func PostJSON(ctx context.Context, target string, cred auth.RequestCredential, body any, client *stdhttp.Client) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}

	req, err := newRequest(ctx, stdhttp.MethodPost, target, bytes.NewReader(payload), "application/json", cred)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return send(req, client, 0)
}

// Builds a request with the standard headers and credentials applied.
func newRequest(ctx context.Context, method, target string, body io.Reader, accept string, cred auth.RequestCredential) (*stdhttp.Request, error) {
	req, err := NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", accept)
	if cred != nil {
		if err := cred.Set(req); err != nil {
			return nil, err
		}
	}
	return req, nil
}

// Executes a prepared request and returns the body of a 2xx response.
func send(req *stdhttp.Request, client *stdhttp.Client, maxBodyBytes int64) ([]byte, error) {
	if client == nil {
		client = DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bodyReader io.Reader = resp.Body
	if maxBodyBytes > 0 {
		readLimit := maxBodyBytes
		if readLimit < int64(^uint64(0)>>1) {
			readLimit++
		}
		bodyReader = io.LimitReader(resp.Body, readLimit)
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}
	if maxBodyBytes > 0 && int64(len(body)) > maxBodyBytes {
		return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", maxBodyBytes)
	}

	if resp.StatusCode < stdhttp.StatusOK || resp.StatusCode >= stdhttp.StatusMultipleChoices {
		return nil, errors.Join(
			errors.ErrRequestFailed,
			errors.Fmt("%s: %s", resp.Status, strings.TrimSpace(string(body))),
		)
	}
	return body, nil
}
