package auth

//go:generate enumify -names typeNames

// Type represents the kind of auth credentials used to authenticate a
// client.
type Type uint8

const (
	TypeUnknown            Type = iota // unknown authentication type
	TypeAPIKey                         // API key
	TypeToken                          // token
	TypeBasic                          // basic authentication
	TypeOAuth2Client                   // OAuth 2.0 client credentials
	TypeOAuth2Token                    // complete OAuth 2.0 token
	TypeOpenAIOrganization             // OpenAI organization
	TypeNone                           // no authentication required
)

var typeNames = []string{
	"unknown",
	"api_key",
	"token",
	"basic",
	"oauth2_client",
	"oauth2_token",
	"oai_organization",
	"none",
}
