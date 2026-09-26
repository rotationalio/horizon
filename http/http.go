package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/version"
)

// Export selected standard-library functions so callers do not need to import
// net/http just to construct requests through Horizon's client.
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

// Sets HTTP authentication headers on req from cred.
func ApplyCreds(req *stdhttp.Request, cred auth.Credential) error {
	if cred == nil {
		return nil
	}

	switch cred.Type() {
	case auth.TypeNone:
		return nil
	case auth.TypeAPIKey:
		apiKey, err := cred.(auth.APIKey).APIKey()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case auth.TypeOpenAIOrganization:
		apiKey, organization, project, err := cred.(auth.OpenAIOrganization).OpenAIOrganization()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if organization != "" {
			req.Header.Set("OpenAI-Organization", organization)
		}
		if project != "" {
			req.Header.Set("OpenAI-Project", project)
		}
	default:
		return errors.Join(errors.ErrUnsupportedCredentialType, errors.Fmt("credential type: %s", cred.Type()))
	}

	return nil
}

// Performs an authenticated GET request and returns the response body.
func Get(ctx context.Context, url string, cred auth.Credential) ([]byte, error) {
	req, err := NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", fmt.Sprintf("Horizon/%s", version.String(true)))
	req.Header.Set("Accept", "application/json")

	if err = ApplyCreds(req, cred); err != nil {
		return nil, err
	}

	resp, err := DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != stdhttp.StatusOK {
		return nil, errors.Join(
			errors.ErrRequestFailed,
			errors.Fmt("%s: %s", resp.Status, strings.TrimSpace(string(body))),
		)
	}

	return body, nil
}

// Performs an authenticated GET against an endpoint, optionally appending a
// suffix as an additional path component.
func GetSuffix(ctx context.Context, baseEP, suffix string, cred auth.Credential) ([]byte, error) {
	if baseEP == "" {
		return nil, errors.ErrBaseEndpointRequired
	}

	url := strings.TrimRight(baseEP, "/")
	if suffix != "" {
		url += "/" + strings.TrimPrefix(suffix, "/")
	}

	return Get(ctx, url, cred)
}
