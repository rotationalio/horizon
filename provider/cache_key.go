package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
	"golang.org/x/oauth2"
)

// providerCacheKey identifies a cached provider client without comparing credential pointers.
type providerCacheKey struct {
	ID                ulid.ULID
	APIType           types.APIType
	ProviderType      types.ProviderType
	InferenceEndpoint string
	CatalogEndpoint   string
	DefaultModel      string
	CredentialType    auth.Type
	Credential        string
}

func providerCacheKeyFrom(conf config.Provider) (providerCacheKey, error) {
	credValue, err := credentialFingerprint(conf.Credentials)
	if err != nil {
		return providerCacheKey{}, err
	}

	return providerCacheKey{
		ID:                conf.ID,
		APIType:           conf.APIType,
		ProviderType:      conf.ProviderType,
		InferenceEndpoint: conf.InferenceEndpoint,
		CatalogEndpoint:   conf.CatalogEndpoint,
		DefaultModel:      conf.DefaultModel,
		CredentialType:    conf.Credentials.Type(),
		Credential:        credValue,
	}, nil
}

// credentialFingerprint returns a stable credential identity for cache keys.
func credentialFingerprint(c auth.Credential) (string, error) {
	if c == nil {
		return "", errors.ErrInvalidCredentials
	}

	var (
		value string
		err   error
	)
	switch c.Type() {
	case auth.TypeNone:
		value = ""
	case auth.TypeAPIKey:
		key, ok := c.(auth.APIKey)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		value, err = key.APIKey()
	case auth.TypeToken:
		token, ok := c.(auth.Token)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		value, err = token.Token()
	case auth.TypeBasic:
		basic, ok := c.(auth.Basic)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		var username, password string
		username, password, err = basic.Basic()
		value = strings.Join([]string{username, password}, "\x00")
	case auth.TypeOAuth2Client:
		oauth, ok := c.(auth.OAuth2Client)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		var clientID, clientSecret string
		clientID, clientSecret, err = oauth.OAuth2Client()
		value = strings.Join([]string{clientID, clientSecret}, "\x00")
	case auth.TypeOAuth2Token:
		token, ok := c.(auth.OAuth2Token)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		var tokenValue *oauth2.Token
		tokenValue, err = token.OAuth2Token()
		if tokenValue != nil {
			value = strings.Join([]string{tokenValue.AccessToken, tokenValue.RefreshToken, tokenValue.TokenType, tokenValue.Expiry.String()}, "\x00")
		}
	case auth.TypeOpenAIOrganization:
		org, ok := c.(auth.OpenAIOrganization)
		if !ok {
			err = errors.ErrInvalidCredentials
			break
		}
		var apiKey, organization, project string
		apiKey, organization, project, err = org.OpenAIOrganization()
		value = strings.Join([]string{apiKey, organization, project}, "\x00")
	default:
		err = fmt.Errorf("%w: unsupported credential type %q", errors.ErrInvalidCredentials, c.Type())
	}

	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), err
}
