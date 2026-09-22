package http

import (
	"context"

	"io"
	"net/http"
	"strings"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
)

// DefaultClient is the HTTP client used by Horizon provider helpers.
var DefaultClient = http.DefaultClient

// ApplyCreds sets HTTP authentication headers on req from cred.
func ApplyCreds(req *http.Request, cred auth.Credential) error {
	if cred == nil || cred.(auth.None).None() {
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

// Get performs an authenticated GET request and returns the response body.
func Get(ctx context.Context, url string, cred auth.Credential) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Horizon")
	req.Header.Set("Accept", "application/json")

	if err = ApplyCreds(req, cred); err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Join(
			errors.ErrRequestFailed,
			errors.Fmt("%s: %s", resp.Status, strings.TrimSpace(string(body))),
		)
	}

	return body, nil
}

// CatalogGet performs an authenticated GET against a catalog endpoint.
// An optional path suffix is appended to catalogEndpoint.
func CatalogGet(ctx context.Context, catalogEndpoint, pathSuffix string, cred auth.Credential) ([]byte, error) {
	if catalogEndpoint == "" {
		return nil, errors.ErrCatalogEndpointRequired
	}

	url := strings.TrimRight(catalogEndpoint, "/")
	if pathSuffix != "" {
		url += "/" + strings.TrimPrefix(pathSuffix, "/")
	}

	return Get(ctx, url, cred)
}
