package horizon

import (
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/provider"
	providerapi "go.rtnl.ai/horizon/provider/api"
	providerconfig "go.rtnl.ai/horizon/provider/config"
)

// The root-level alias for Horizon runtime configuration.
type Config = config.Config

// Provider is the core provider client used by Horizon integrations.
type Provider = provider.Provider

// ProviderConfig configures a provider client.
type ProviderConfig = providerconfig.Provider

// Request and Response are the provider-facing inference contracts.
type Request = providerapi.Request
type Response = providerapi.Response
