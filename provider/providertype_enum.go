package provider

//go:generate enumify -names providerTypeNames

// ProviderType identifies the external provider (catalog, pricing, usage, account behavior).
type ProviderType uint8

const (
	ProviderTypeUnknown ProviderType = iota
	ProviderTypeMock
	ProviderTypeOpenAI
	ProviderTypeOpenAICompatible // exactly like OpenAI, but may have different endpoints
	ProviderTypeOpenRouter
)

var providerTypeNames = [][]string{
	{
		"unknown",
		"mock",
		"openai",
		"openai_compat",
		"openrouter",
	},
	{
		// Display Name
		"Unknown",
		"Mock",
		"OpenAI",
		"OpenAI Compatible",
		"OpenRouter",
	},
	// URLs for ProviderTypes: builds on each other from base to endpoints.
	//
	// NOTE: Ensure that there is a closing slash but no opening slash.
	{
		// BaseURL
		"",                             // unknown
		"",                             // mock
		"https://openai.com/",          // openai
		"https://replace.example.com/", // openai_compat
		"https://openrouter.ai/",       // openrouter
	},
	{
		// Informational Link (BaseURL + this)
		"", // unknown
		"", // mock
		"", // openai
		"", // openai_compat
		"", // openrouter
	},
	{
		// Endpoint (BaseURL + this)
		"",        // unknown
		"",        // mock
		"api/v1/", // openai
		"api/v1/", // openai_compat
		"api/v1/", // openrouter
	},
	{
		// CatalogEndpoint (Endpoint + this)
		"",        // unknown
		"",        // mock
		"models/", // openai
		"models/", // openai_compat
		"models/", // openrouter
	},
}
