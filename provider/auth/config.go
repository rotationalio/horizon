package auth

import (
	"time"

	"go.rtnl.ai/confire"
)

// A confire-compatible struct for loading a [Credentials] from environment
// variables.
type CredentialsConfig struct {
	Type         string    `desc:"the type of credentials being described in the environment variables"`
	APIKey       string    `split_words:"false" desc:"required for api key authentication"`
	Organization string    `desc:"required for openai organization authentication"`
	Project      string    `desc:"required for openai organization authentication"`
	Token        string    `desc:"required for token authentication"`
	Username     string    `desc:"required for basic authentication"`
	Password     string    `desc:"required for basic authentication"`
	ClientID     string    `split_words:"true" desc:"required for oauth2 client authentication"`
	ClientSecret string    `split_words:"true" desc:"required for oauth2 client authentication"`
	AccessToken  string    `split_words:"true" desc:"required for oauth2 token authentication"`
	RefreshToken string    `split_words:"true" desc:"optional for oauth2 token authentication"`
	TokenType    string    `split_words:"true" desc:"optional for oauth2 token authentication"`
	Expiry       time.Time `split_words:"true" desc:"optional for oauth2 token authentication"`
}

func (c *CredentialsConfig) Credentials() *Credentials {
	authType, _ := ParseType(c.Type)
	creds := &Credentials{
		wire: credswire{
			Type:         authType,
			APIKey:       c.APIKey,
			Organization: c.Organization,
			Project:      c.Project,
			Token:        c.Token,
			Username:     c.Username,
			Password:     c.Password,
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			AccessToken:  c.AccessToken,
			RefreshToken: c.RefreshToken,
			TokenType:    c.TokenType,
			Expiry:       c.Expiry,
		},
	}
	creds.Normalize()
	return creds
}

// Returns confire validation errors for configuration via the environment.
func (c *CredentialsConfig) Validate(conf string) (err error) {
	// Empty credentials are valid.
	if c.IsZero() {
		return nil
	}

	var (
		perr     error
		authType Type
	)

	if authType, perr = ParseType(c.Type); perr != nil {
		err = confire.Join(err, confire.Invalid("", "type", perr.Error()))
	}

	switch authType {
	case TypeAPIKey:
		if c.APIKey == "" {
			err = confire.Join(err, confire.Required(conf, "apiKey"))
		}
	case TypeToken:
		if c.Token == "" {
			err = confire.Join(err, confire.Required(conf, "token"))
		}
	case TypeBasic:
		if c.Username == "" {
			err = confire.Join(err, confire.Required(conf, "username"))
		}
		if c.Password == "" {
			err = confire.Join(err, confire.Required(conf, "password"))
		}
	case TypeOAuth2Client:
		if c.ClientID == "" {
			err = confire.Join(err, confire.Required(conf, "clientID"))
		}
		if c.ClientSecret == "" {
			err = confire.Join(err, confire.Required(conf, "clientSecret"))
		}
	case TypeOAuth2Token:
		if c.AccessToken == "" {
			err = confire.Join(err, confire.Required(conf, "accessToken"))
		}
	case TypeOpenAIOrganization:
		if c.Organization == "" {
			err = confire.Join(err, confire.Required(conf, "organization"))
		}
		if c.Project == "" {
			err = confire.Join(err, confire.Required(conf, "project"))
		}
	default:
		err = confire.Join(err, confire.Invalid(conf, "type", "must be a supported auth type"))
	}

	return err
}

// IsZero returns true if the configuration is empty.
func (c *CredentialsConfig) IsZero() bool {
	return c.Type == "" &&
		c.APIKey == "" &&
		c.Organization == "" &&
		c.Project == "" &&
		c.Token == "" &&
		c.Username == "" &&
		c.Password == "" &&
		c.ClientID == "" &&
		c.ClientSecret == "" &&
		c.AccessToken == "" &&
		c.RefreshToken == "" &&
		c.TokenType == "" &&
		c.Expiry.IsZero()
}
