package provider

import "go.rtnl.ai/horizon/provider/auth"

//=============================================================================
// Provider Defaults
//=============================================================================

// Constants for provider type indices.
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

// Returns the BaseURL for this provider type. If the provider type does not
// have a default BaseURL, returns the empty string.
func (p ProviderType) BaseURL() string {
	return providerTypeNames[provIdxBaseURL][p]
}

// Returns the informational URL for this provider type. If the provider type
// does not have a default Link, returns the empty string.
func (p ProviderType) Link() string {
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxLink][p]
}

// Returns the endpoint URL for this provider type. If the provider type does
// not have an endpoint, returns the empty string.
func (p ProviderType) Endpoint() string {
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxEndpoint][p]
}

// Returns the catalog URL for this provider type. If the provider type does not
// have a catalog endpoint, returns the empty string.
func (p ProviderType) CatalogURL() string {
	return providerTypeNames[provIdxBaseURL][p] + providerTypeNames[provIdxEndpoint][p] + providerTypeNames[provIdxCatalog][p]
}

// Returns the API types supported by the registered provider. The first entry
// is the provider's default API type.
func (p ProviderType) SupportedAPITypes() []APIType {
	if registration, exists := LookupRegistration(p); exists {
		return registration.APITypes
	}
	return []APIType{APITypeUnknown}
}

// Returns the auth types supported by the registered provider.
func (p ProviderType) SupportedAuthTypes() []auth.Type {
	if registration, exists := LookupRegistration(p); exists {
		return registration.AuthTypes
	}
	return []auth.Type{auth.TypeUnknown}
}
