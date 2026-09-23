package auth

import "golang.org/x/oauth2"

// Credential defines the common interface for all supported client credential
// [Type]s.
type Credential interface {
	// Type returns the credential's [Type], indicating which authentication
	// mechanism is used.
	Type() Type
	// Validate checks that the credential is well-formed for its [Type] (e.g.,
	// nonempty secrets).
	Validate() error
}

// None represents "no credentials required" (credentials type [TypeNone]) for
// endpoints that do not need authentication.
type None interface {
	Credential
	// None returns true if the credential is intentionally empty, indicating
	// unauthenticated access.
	None() bool
}

// APIKey is for services that require a static API key (credentials type
// [TypeAPIKey]), commonly provided in an HTTP header.
type APIKey interface {
	Credential
	// APIKey returns the API key string used for authentication.
	APIKey() (string, error)
}

// Token represents bearer token authentication (credentials type [TypeToken]),
// such as OAuth2 access tokens or JWT tokens.
type Token interface {
	Credential
	// Token returns the bearer token string for use in authentication headers.
	Token() (string, error)
}

// Basic covers HTTP Basic Auth (credentials type [TypeBasic]), with a username
// and password.
type Basic interface {
	Credential
	// Basic returns the username and password for HTTP basic authentication.
	Basic() (username, password string, err error)
}

// OAuth2Client is used for OAuth2 "client credentials" flow ([TypeOAuth2Client]),
// providing a client ID and secret.
type OAuth2Client interface {
	Credential
	// OAuth2Client returns the OAuth2 client ID and secret.
	OAuth2Client() (clientID, clientSecret string, err error)
}

// OAuth2Token represents a complete OAuth2 token, including its access token,
// optional refresh token, token type, and expiry.
type OAuth2Token interface {
	Credential
	// OAuth2Token returns the complete OAuth2 token.
	OAuth2Token() (*oauth2.Token, error)
}

// OpenAIOrganization is for OpenAI Organization authentication
// ([TypeOpenAIOrganization]), which includes an API key and optional
// organization and project identifiers for fine-grained API access control.
type OpenAIOrganization interface {
	Credential
	// OpenAIOrganization returns the API key, organization, and project strings for OpenAI authentication.
	OpenAIOrganization() (apiKey, organization, project string, err error)
}

// TODO: specify workload identity provider credentials.
