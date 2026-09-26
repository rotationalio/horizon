package auth_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"golang.org/x/oauth2"
)

// Verifies every credential variant accepts required fields, rejects missing
// fields, and reports validation paths consistently.
func TestCredentialsValidate(t *testing.T) {
	t.Run("MissingDTO", func(t *testing.T) {
		var creds *auth.Credentials
		errors.RequireValidationFields(t, creds.Validate(), "credentials")
	})

	t.Run("MissingDTOCustomFieldPath", func(t *testing.T) {
		var creds *auth.Credentials
		errors.RequireValidationFields(t, creds.ValidateFor("integration.credentials"), "integration.credentials")
	})

	t.Run("BlankCustomFieldPathFallsBackToCredentials", func(t *testing.T) {
		var creds *auth.Credentials
		errors.RequireValidationFields(t, creds.ValidateFor("   "), "credentials")
	})

	t.Run("NoneValid", func(t *testing.T) {
		require.NoError(t, auth.NewNone().Validate())
	})

	t.Run("APIKeyValid", func(t *testing.T) {
		creds := auth.NewAPIKey("secret")
		require.NoError(t, creds.Validate())
	})

	t.Run("APIKeyValidTrimsWhitespace", func(t *testing.T) {
		creds := auth.NewAPIKey("  secret  ")
		require.NoError(t, creds.Validate())
		key, err := creds.APIKey()
		require.NoError(t, err)
		require.Equal(t, "secret", key)
	})

	t.Run("MissingAPIKey", func(t *testing.T) {
		creds := auth.NewAPIKey("")
		errors.RequireValidationFields(t, creds.Validate(), "api_key")
	})

	t.Run("TokenValid", func(t *testing.T) {
		creds := auth.NewToken("token-value")
		require.NoError(t, creds.Validate())
	})

	t.Run("MissingToken", func(t *testing.T) {
		creds := auth.NewToken("")
		errors.RequireValidationFields(t, creds.Validate(), "token")
	})

	t.Run("BasicValid", func(t *testing.T) {
		creds := auth.NewBasic("username", "password")
		require.NoError(t, creds.Validate())
	})

	t.Run("BasicMissingFields", func(t *testing.T) {
		creds := auth.NewBasic("", "")
		errors.RequireValidationFields(t, creds.Validate(), "username", "password")
	})

	t.Run("OAuth2ClientValid", func(t *testing.T) {
		creds := auth.NewOAuth2Client("client-id", "client-secret")
		require.NoError(t, creds.Validate())
	})

	t.Run("OAuth2ClientMissingFields", func(t *testing.T) {
		creds := auth.NewOAuth2Client("", "")
		errors.RequireValidationFields(t, creds.Validate(), "client_id", "client_secret")
	})

	t.Run("OAuth2TokenValid", func(t *testing.T) {
		creds := auth.NewOAuth2Token(&oauth2.Token{AccessToken: "access-token", RefreshToken: "refresh-token"})
		require.NoError(t, creds.Validate())
	})

	t.Run("OAuth2TokenMissingAccessToken", func(t *testing.T) {
		creds := auth.NewOAuth2Token(&oauth2.Token{RefreshToken: "refresh-token"})
		errors.RequireValidationFields(t, creds.Validate(), "access_token")
	})

	t.Run("OpenAIOrganizationValid", func(t *testing.T) {
		creds := auth.NewOpenAIOrganization("key", "org", "project")
		require.NoError(t, creds.Validate())
	})

	t.Run("OpenAIOrganizationMissingFields", func(t *testing.T) {
		creds := auth.NewOpenAIOrganization("", "", "")
		errors.RequireValidationFields(t, creds.Validate(), "api_key", "organization", "project")
	})

	t.Run("UnknownType", func(t *testing.T) {
		creds := &auth.Credentials{}
		require.NoError(t, creds.Validate())
		errors.RequireValidationFields(t, creds.ValidateFor("auth"), "type")
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		creds := auth.NewForTest(auth.Type(255))
		errors.RequireValidationFields(t, creds.Validate(), "type")
	})
}

// Verifies user-controlled credential fields are trimmed without changing the
// credential variant or semantic values.
func TestCredentialsNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   *auth.Credentials
		want *auth.Credentials
	}{
		{
			name: "APIKey",
			in:   auth.NewAPIKey("  secret  "),
			want: auth.NewAPIKey("secret"),
		},
		{
			name: "Token",
			in:   auth.NewToken("  token-value  "),
			want: auth.NewToken("token-value"),
		},
		{
			name: "Basic",
			in:   auth.NewBasic("  user  ", "  pass  "),
			want: auth.NewBasic("user", "pass"),
		},
		{
			name: "OAuth2Client",
			in:   auth.NewOAuth2Client("  client-id  ", "  client-secret  "),
			want: auth.NewOAuth2Client("client-id", "client-secret"),
		},
		{
			name: "OAuth2Token",
			in:   auth.NewOAuth2Token(&oauth2.Token{AccessToken: "  access-token  ", RefreshToken: "  refresh-token  ", TokenType: "  Bearer  "}),
			want: auth.NewOAuth2Token(&oauth2.Token{AccessToken: "access-token", RefreshToken: "refresh-token", TokenType: "Bearer"}),
		},
		{
			name: "OpenAIOrganization",
			in:   auth.NewOpenAIOrganization("  key  ", "  org  ", "  project  "),
			want: auth.NewOpenAIOrganization("key", "org", "project"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Normalize()
			require.Equal(t, tc.want.Type(), tc.in.Type())
			assertCredentialFieldsEqual(t, tc.want, tc.in)
		})
	}
}

// Verifies credential type reporting for populated and nil credentials.
func TestCredentialsType(t *testing.T) {
	require.Equal(t, auth.TypeAPIKey, auth.NewAPIKey("k").Type())
	require.Equal(t, auth.TypeUnknown, (*auth.Credentials)(nil).Type())
}

// Verifies credentials preserve their variant and values through JSON
// serialization.
func TestCredentialsJSONRoundTrip(t *testing.T) {
	orig := auth.NewOpenAIOrganization("key", "org", "project")
	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var got auth.Credentials
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, orig.Type(), got.Type())

	key, org, project, err := got.OpenAIOrganization()
	require.NoError(t, err)
	wantKey, wantOrg, wantProject, err := orig.OpenAIOrganization()
	require.NoError(t, err)
	require.Equal(t, wantKey, key)
	require.Equal(t, wantOrg, org)
	require.Equal(t, wantProject, project)
}

func assertCredentialFieldsEqual(t *testing.T, want, got *auth.Credentials) {
	t.Helper()

	switch want.Type() {
	case auth.TypeAPIKey:
		wantKey, err := want.APIKey()
		require.NoError(t, err)
		gotKey, err := got.APIKey()
		require.NoError(t, err)
		require.Equal(t, wantKey, gotKey)
	case auth.TypeToken:
		wantToken, err := want.Token()
		require.NoError(t, err)
		gotToken, err := got.Token()
		require.NoError(t, err)
		require.Equal(t, wantToken, gotToken)
	case auth.TypeBasic:
		wantUser, wantPass, err := want.Basic()
		require.NoError(t, err)
		gotUser, gotPass, err := got.Basic()
		require.NoError(t, err)
		require.Equal(t, wantUser, gotUser)
		require.Equal(t, wantPass, gotPass)
	case auth.TypeOAuth2Client:
		wantID, wantSecret, err := want.OAuth2Client()
		require.NoError(t, err)
		gotID, gotSecret, err := got.OAuth2Client()
		require.NoError(t, err)
		require.Equal(t, wantID, gotID)
		require.Equal(t, wantSecret, gotSecret)
	case auth.TypeOAuth2Token:
		wantToken, err := want.OAuth2Token()
		require.NoError(t, err)
		gotToken, err := got.OAuth2Token()
		require.NoError(t, err)
		require.Equal(t, wantToken, gotToken)
	case auth.TypeOpenAIOrganization:
		wantKey, wantOrg, wantProject, err := want.OpenAIOrganization()
		require.NoError(t, err)
		gotKey, gotOrg, gotProject, err := got.OpenAIOrganization()
		require.NoError(t, err)
		require.Equal(t, wantKey, gotKey)
		require.Equal(t, wantOrg, gotOrg)
		require.Equal(t, wantProject, gotProject)
	}
}
