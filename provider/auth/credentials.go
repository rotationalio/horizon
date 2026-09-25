package auth

import (
	"encoding/json"
	"strings"
	"time"

	"go.rtnl.ai/x/validation"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// An implementation of [Credential]. Create a new [Credentials] using auth.New*
// functions.
type Credentials struct {
	wire credswire
}

var _ Credential = (*Credentials)(nil)

//=============================================================================
// Constructors
//=============================================================================

// None returns [Credentials] for no authentication.
func NewNone() *Credentials {
	return &Credentials{wire: credswire{
		Type: TypeNone,
	}}
}

// APIKey returns [Credentials] for API key
func NewAPIKey(apiKey string) *Credentials {
	return &Credentials{wire: credswire{
		Type:   TypeAPIKey,
		APIKey: apiKey,
	}}
}

// Token returns [Credentials] for bearer token
func NewToken(token string) *Credentials {
	return &Credentials{wire: credswire{
		Type:  TypeToken,
		Token: token,
	}}
}

// Basic returns [Credentials] for HTTP basic
func NewBasic(username, password string) *Credentials {
	return &Credentials{wire: credswire{
		Type:     TypeBasic,
		Username: username,
		Password: password,
	}}
}

// OAuth2Client returns [Credentials] for OAuth2 client credentials.
func NewOAuth2Client(clientID, clientSecret string) *Credentials {
	return &Credentials{wire: credswire{
		Type:         TypeOAuth2Client,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}}
}

// OAuth2Token returns [Credentials] for a complete OAuth2 token.
func NewOAuth2Token(token *oauth2.Token) *Credentials {
	if token == nil {
		return &Credentials{}
	}
	return &Credentials{wire: credswire{
		Type:         TypeOAuth2Token,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}}
}

// OpenAIOrganization returns [Credentials] for OpenAI organization/project
func NewOpenAIOrganization(apiKey, organization, project string) *Credentials {
	return &Credentials{wire: credswire{
		Type:         TypeOpenAIOrganization,
		APIKey:       apiKey,
		Organization: organization,
		Project:      project,
	}}
}

//=============================================================================
// Serialization
//=============================================================================

// MarshalJSON returns a mapping of the underlying credential data without exposing it to
// external access or modification using an internal type. Only the values that are present
// are marshaled and all others excluded or ignored preserving the Union type properties.
func (c *Credentials) MarshalJSON() ([]byte, error) {
	if c == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(c.wire)
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Credentials) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = Credentials{}
		return nil
	}
	return json.Unmarshal(data, &c.wire)
}

// MarshalYAML implements yaml.Marshaler.
func (c *Credentials) MarshalYAML() (any, error) {
	if c == nil {
		return nil, nil
	}
	return c.wire, nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *Credentials) UnmarshalYAML(value *yaml.Node) error {
	return value.Decode(&c.wire)
}

//=============================================================================
// Normalization
//=============================================================================

// Normalize trims user-controlled credential fields.
func (c *Credentials) Normalize() {
	if c == nil {
		return
	}

	w := &c.wire
	w.APIKey = strings.TrimSpace(w.APIKey)
	w.Organization = strings.TrimSpace(w.Organization)
	w.Project = strings.TrimSpace(w.Project)
	w.Token = strings.TrimSpace(w.Token)
	w.Username = strings.TrimSpace(w.Username)
	w.Password = strings.TrimSpace(w.Password)
	w.ClientID = strings.TrimSpace(w.ClientID)
	w.ClientSecret = strings.TrimSpace(w.ClientSecret)
	w.RefreshToken = strings.TrimSpace(w.RefreshToken)
	w.AccessToken = strings.TrimSpace(w.AccessToken)
	w.TokenType = strings.TrimSpace(w.TokenType)
}

//=============================================================================
// Validation
//=============================================================================

// Validates a credentials object.
func (c *Credentials) Validate() (err error) {
	if c == nil {
		return validation.Error(err, validation.MissingField("credentials"))
	}
	if c.Zero() {
		return nil
	}
	return c.ValidateFor("credentials")
}

// Validates a credentials object at given field path (e.g. "credentials",
// "provider.credentials", etc).
func (c *Credentials) ValidateFor(field string) (err error) {
	field = strings.TrimSpace(field)
	if field == "" {
		field = "credentials"
	}

	if c == nil {
		return validation.Error(err, validation.MissingField(field))
	}

	c.Normalize()

	w := &c.wire

	switch w.Type {
	case TypeAPIKey:
		if w.APIKey == "" {
			err = validation.Error(err, validation.MissingField("api_key"))
		}
	case TypeToken:
		if w.Token == "" {
			err = validation.Error(err, validation.MissingField("token"))
		}
	case TypeBasic:
		if w.Username == "" {
			err = validation.Error(err, validation.MissingField("username"))
		}
		if w.Password == "" {
			err = validation.Error(err, validation.MissingField("password"))
		}
	case TypeOAuth2Client:
		if w.ClientID == "" {
			err = validation.Error(err, validation.MissingField("client_id"))
		}
		if w.ClientSecret == "" {
			err = validation.Error(err, validation.MissingField("client_secret"))
		}
	case TypeOAuth2Token:
		if w.AccessToken == "" {
			err = validation.Error(err, validation.MissingField("access_token"))
		}
	case TypeOpenAIOrganization:
		if w.Organization == "" {
			err = validation.Error(err, validation.MissingField("organization"))
		}
		if w.Project == "" {
			err = validation.Error(err, validation.MissingField("project"))
		}
	default:
		err = validation.Error(err, validation.IncorrectField("type", "must be a supported auth type"))
	}

	return err
}

//=============================================================================
// Type and predicates
//=============================================================================

// Type returns the credential auth type.
func (c *Credentials) Type() Type {
	if c == nil {
		return TypeUnknown
	}
	return c.wire.Type
}

// Zero returns true if the normalized Credentials are empty.
func (c *Credentials) Zero() bool {
	c.Normalize()
	w := &c.wire
	return w.Type == TypeUnknown &&
		w.APIKey == "" &&
		w.Organization == "" &&
		w.Project == "" &&
		w.Token == "" &&
		w.Username == "" &&
		w.Password == "" &&
		w.ClientID == "" &&
		w.ClientSecret == "" &&
		w.AccessToken == "" &&
		w.RefreshToken == "" &&
		w.TokenType == "" &&
		w.Expiry.IsZero()
}

//=============================================================================
// Accessors
//=============================================================================

// None returns true if no authentication is required.
func (c *Credentials) None() bool {
	if c == nil {
		return false
	}
	return c.wire.Type == TypeNone
}

// APIKey returns the API key if the [Credentials.Type] matches
// [TypeAPIKey], and the API key is not empty, or an error otherwise.
func (c *Credentials) APIKey() (string, error) {
	if c == nil {
		return "", ErrNil
	}
	w := &c.wire
	if w.Type != TypeAPIKey {
		return "", ErrWrongType
	}
	if w.APIKey == "" {
		return "", ErrMissingValue
	}
	return w.APIKey, nil
}

// Token returns the token if the [Credentials.Type] matches [TypeToken],
// and the token is not empty, or an error otherwise.
func (c *Credentials) Token() (string, error) {
	if c == nil {
		return "", ErrNil
	}
	w := &c.wire
	if w.Type != TypeToken {
		return "", ErrWrongType
	}
	if w.Token == "" {
		return "", ErrMissingValue
	}
	return w.Token, nil
}

// Basic returns the username and password if the [Credentials.Type] matches
// [TypeBasic], and the username and password are not empty, or an error otherwise.
func (c *Credentials) Basic() (username, password string, err error) {
	if c == nil {
		return "", "", ErrNil
	}
	w := &c.wire
	if w.Type != TypeBasic {
		return "", "", ErrWrongType
	}
	if w.Username == "" || w.Password == "" {
		return "", "", ErrMissingValue
	}
	return w.Username, w.Password, nil
}

// OAuth2Client returns the client ID and secret if the [Credentials.Type] matches
// [TypeOAuth2Client], and the client ID and secret are not empty, or an error otherwise.
func (c *Credentials) OAuth2Client() (clientID, clientSecret string, err error) {
	if c == nil {
		return "", "", ErrNil
	}
	w := &c.wire
	if w.Type != TypeOAuth2Client {
		return "", "", ErrWrongType
	}
	if w.ClientID == "" || w.ClientSecret == "" {
		return "", "", ErrMissingValue
	}
	return w.ClientID, w.ClientSecret, nil
}

// OAuth2Token returns the complete OAuth2 token if the credential has
// [TypeOAuth2Token].
func (c *Credentials) OAuth2Token() (*oauth2.Token, error) {
	if c == nil {
		return nil, ErrNil
	}
	w := &c.wire
	if w.Type != TypeOAuth2Token {
		return nil, ErrWrongType
	}
	if w.AccessToken == "" {
		return nil, ErrMissingValue
	}
	return &oauth2.Token{
		AccessToken:  w.AccessToken,
		RefreshToken: w.RefreshToken,
		TokenType:    w.TokenType,
		Expiry:       w.Expiry,
	}, nil
}

// OpenAIOrganization returns the API key, organization, and project if the
// [Credentials.Type] matches [TypeOpenAIOrganization], and the organization
// and project are not empty, and the API key is not empty, or an error otherwise.
func (c *Credentials) OpenAIOrganization() (apiKey, organization, project string, err error) {
	if c == nil {
		return "", "", "", ErrNil
	}
	w := &c.wire
	if w.Type != TypeOpenAIOrganization {
		return "", "", "", ErrWrongType
	}
	if w.APIKey == "" || w.Organization == "" || w.Project == "" {
		return "", "", "", ErrMissingValue
	}
	return w.APIKey, w.Organization, w.Project, nil
}

//=============================================================================
// Private
//=============================================================================

// Hides the internal details of the credentials from the public API while
// allowing marshalling and unmarshaling to/from JSON and YAML.
type credswire struct {
	Type         Type      `json:"type" yaml:"type"`
	APIKey       string    `json:"api_key,omitempty" yaml:"apikey,omitempty"`
	Organization string    `json:"organization,omitempty" yaml:"organization,omitempty"`
	Project      string    `json:"project,omitempty" yaml:"project,omitempty"`
	Token        string    `json:"token,omitempty" yaml:"token,omitempty"`
	Username     string    `json:"username,omitempty" yaml:"username,omitempty"`
	Password     string    `json:"password,omitempty" yaml:"password,omitempty"`
	ClientID     string    `json:"client_id,omitempty" yaml:"clientID,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty" yaml:"clientSecret,omitempty"`
	AccessToken  string    `json:"access_token,omitempty" yaml:"accessToken,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty" yaml:"refreshToken,omitempty"`
	TokenType    string    `json:"token_type,omitempty" yaml:"tokenType,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty" yaml:"expiry,omitempty"`
}
