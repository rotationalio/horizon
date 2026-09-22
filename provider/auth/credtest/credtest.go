// Package credtest provides bare-bones [auth.Credential] values for horizon tests.
package credtest

import (
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"golang.org/x/oauth2"
)

var _ auth.Credential = (*TestCredential)(nil)

// TestCredential is a minimal auth.Credential for horizon package tests.
type TestCredential struct {
	typ          auth.Type
	apiKey       string
	organization string
	project      string
	token        string
	username     string
	password     string
	clientID     string
	clientSecret string
	oauthToken   *oauth2.Token
}

// --- Constructors ---

// APIKey returns test credentials for API key auth.
func APIKey(apiKey string) *TestCredential {
	return &TestCredential{typ: auth.TypeAPIKey, apiKey: apiKey}
}

// Token returns test credentials for bearer token auth.
func Token(token string) *TestCredential {
	return &TestCredential{typ: auth.TypeToken, token: token}
}

// Basic returns test credentials for HTTP basic auth.
func Basic(username, password string) *TestCredential {
	return &TestCredential{typ: auth.TypeBasic, username: username, password: password}
}

// OAuth2Client returns test credentials for OAuth2 client credentials.
func OAuth2Client(clientID, clientSecret string) *TestCredential {
	return &TestCredential{typ: auth.TypeOAuth2Client, clientID: clientID, clientSecret: clientSecret}
}

// OAuth2Token returns test credentials for a complete OAuth2 token.
func OAuth2Token(token *oauth2.Token) *TestCredential {
	return &TestCredential{typ: auth.TypeOAuth2Token, oauthToken: token}
}

// OpenAIOrganization returns test credentials for OpenAI organization/project auth.
func OpenAIOrganization(apiKey, organization, project string) *TestCredential {
	return &TestCredential{
		typ:          auth.TypeOpenAIOrganization,
		apiKey:       apiKey,
		organization: organization,
		project:      project,
	}
}

// --- Methods ---

// Type returns the credential auth type.
func (c *TestCredential) Type() auth.Type {
	if c == nil {
		return auth.TypeUnknown
	}
	return c.typ
}

// Validates the test credential has the fields required by its type.
func (c *TestCredential) Validate() error {
	if c == nil || c.typ == auth.TypeUnknown {
		return errors.ErrInvalidCredentials
	}

	switch c.typ {
	case auth.TypeAPIKey, auth.TypeToken:
		if c.apiKey == "" && c.token == "" {
			return errors.ErrInvalidCredentials
		}
	case auth.TypeBasic:
		if c.username == "" || c.password == "" {
			return errors.ErrInvalidCredentials
		}
	case auth.TypeOAuth2Client:
		if c.clientID == "" || c.clientSecret == "" {
			return errors.ErrInvalidCredentials
		}
	case auth.TypeOAuth2Token:
		if c.oauthToken == nil || c.oauthToken.AccessToken == "" {
			return errors.ErrInvalidCredentials
		}
	case auth.TypeOpenAIOrganization:
		if c.apiKey == "" {
			return errors.ErrInvalidCredentials
		}
	}
	return nil
}

// None returns true if no authentication is required.
func (c *TestCredential) None() bool {
	if c == nil {
		return false
	}
	return c.typ == auth.TypeNone
}

func (c *TestCredential) APIKey() (string, error) {
	if c == nil {
		return "", nil
	}
	return c.apiKey, nil
}

func (c *TestCredential) Token() (string, error) {
	if c == nil {
		return "", nil
	}
	return c.token, nil
}

func (c *TestCredential) Basic() (username, password string, err error) {
	if c == nil {
		return "", "", nil
	}
	return c.username, c.password, nil
}

func (c *TestCredential) OAuth2Client() (clientID, clientSecret string, err error) {
	if c == nil {
		return "", "", nil
	}
	return c.clientID, c.clientSecret, nil
}

func (c *TestCredential) OAuth2Token() (*oauth2.Token, error) {
	if c == nil {
		return nil, nil
	}
	return c.oauthToken, nil
}

func (c *TestCredential) OpenAIOrganization() (apiKey, organization, project string, err error) {
	if c == nil {
		return "", "", "", nil
	}
	return c.apiKey, c.organization, c.project, nil
}
