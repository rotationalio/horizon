package provider

import (
	"go.rtnl.ai/horizon/provider/auth"
)

//=============================================================================
// Provider Defaults
//=============================================================================

// Constants for provider type indices
const (
	provIdxDisplay = iota + 1
	provIdxBaseURL
	provIdxLink
	provIdxEndpoint
	provIdxCatalog
)

// Returns a human-friendly provider type name.
func (p ProviderType) DisplayName() string {
	return providerTypeNames[provIdxDisplay][p]
}

// Returns the BaseURL for this provider type. If the provider type
// does not have a default BaseURL, returns the empty string.
func (p ProviderType) BaseURL() string {
	return providerTypeNames[provIdxBaseURL][p]
}

// Returns the informational URL for this provider type. If the provider type
// does not have a default Link, returns the empty string.
func (p ProviderType) Link() string {
	// baseURL + link
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxLink][p]
}

// Returns the endpoint URL for this provider type. If the provider type
// does not have an endpoint, returns the empty string.
func (p ProviderType) Endpoint() string {
	// baseURL + endpoint
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxEndpoint][p]
}

// Returns the catalog URL for this provider type. If the provider type
// does not have a catalog endpoint, returns the empty string.
func (p ProviderType) CatalogURL() string {
	// baseURL + endpoint + catalog
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxEndpoint][p] + providerTypeNames[provIdxCatalog][p]
}

// Returns a list of the supported [APIType] for this provider type.
// The first entry is the default API type for the provider.
func (p ProviderType) SupportedAPITypes() []APIType {
	switch p {
	case ProviderTypeMock:
		// Mock provider supports all API types, even if not all are implemented.
		return []APIType{
			APITypeOpenAIResponses,
			APITypeOpenAIChatCompletions,
		}
	case ProviderTypeOpenAI:
		return []APIType{APITypeOpenAIResponses, APITypeOpenAIChatCompletions}
	case ProviderTypeOpenAICompatible:
		return []APIType{APITypeOpenAIChatCompletions, APITypeOpenAIResponses}
	case ProviderTypeOpenRouter:
		return []APIType{APITypeOpenAIChatCompletions, APITypeOpenAIResponses}
	default:
		return []APIType{APITypeUnknown}
	}
}

// Returns a list of the supported [auth.Type] for this provider type.
func (p ProviderType) SupportedAuthTypes() []auth.Type {
	switch p {
	case ProviderTypeMock:
		// Mock provider supports all auth types, even if not all are implemented.
		return []auth.Type{
			auth.TypeAPIKey,
			auth.TypeToken,
			auth.TypeBasic,
			auth.TypeOAuth2Client,
			auth.TypeOpenAIOrganization,
			auth.TypeNone,
		}
	case ProviderTypeOpenAI, ProviderTypeOpenAICompatible:
		return []auth.Type{auth.TypeAPIKey, auth.TypeOpenAIOrganization}
	case ProviderTypeOpenRouter:
		return []auth.Type{auth.TypeAPIKey}
	default:
		return []auth.Type{auth.TypeUnknown}
	}
}

// IsSingleInstance reports whether only one provider of this type may be
// configured. Additional credentials for a single-instance provider are still
// allowed through the provider credential endpoints.
func (p ProviderType) IsSingleInstance() bool {
	switch p {
	case ProviderTypeOpenAI, ProviderTypeOpenRouter:
		return true
	default:
		// Provider types that are not explicitly listed are assumed to support
		// multiple provider instances so that new custom provider types do not
		// need to change this restriction.
		return false
	}
}
