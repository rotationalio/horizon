package catalog

import (
	"time"

	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/tidal/fields"
)

// Model is the canonical catalog representation of a provider model. It contains
// display metadata normalized from provider wire formats and is the shape
// written to JSONB catalog columns in persistence adapters.
type Model struct {
	//FIXME: we can probably remove this, and allow the app to link a provider to it's catalog, otherwise just make
	// it a pure string and we'll handle it on the other side
	// ProviderType provider.ProviderType `json:"provider_type"`

	// Common display metadata.

	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	Description    string            `json:"description"`
	Author         string            `json:"author,omitempty"`
	Published      time.Time         `json:"published,omitzero"`
	License        string            `json:"license,omitempty"`
	InputModality  modality.Modality `json:"input_modality,omitzero"`
	OutputModality modality.Modality `json:"output_modality,omitzero"`
	Size           string            `json:"size,omitempty"`
	ContextSize    *int32            `json:"context_size,omitempty"`
	InputCost      *float64          `json:"input_cost,omitempty"`  // USD per input token
	OutputCost     *float64          `json:"output_cost,omitempty"` // USD per output token
	TPMLimit       *int32            `json:"tpm_limit,omitempty"`
	RPMLimit       *int32            `json:"rpm_limit,omitempty"`
	IsModerated    *bool             `json:"is_moderated,omitempty"`

	// Model-specific links and metadata.

	Links        Links           `json:"links,omitzero"`
	Architecture Architecture    `json:"architecture,omitzero"`
	Parameters   Parameters      `json:"parameters,omitzero"`
	Capabilities ModelCapability `json:"capabilities,omitzero"`
	Pricing      Pricing         `json:"pricing,omitzero"`
	Limits       Limits          `json:"limits,omitzero"`

	// TODO: figure out a common format for energy usage
	EnergyUsage fields.NullJSONB `json:"energy_usage,omitzero"`

	// Model-specific API, endpoint, and auth overrides. Not commonly used.

	//FIXME: we can probably remove this as it's more of an override for our application? or make it a pure string
	// APIType  provider.APIType `json:"api_type,omitzero"`
	Endpoint string    `json:"endpoint,omitempty"`
	AuthType auth.Type `json:"auth_type,omitzero"`
}

// Links is the list of related resources for a model.
type Links []Link

// Link is one related resource for a model.
type Link struct {
	Tag     string `json:"tag"`
	Display string `json:"display"`
	URL     string `json:"url"`
}

// Architecture is the list of supplemental technical facts for a model.
type Architecture []ArchitectureAttribute

// ArchitectureAttribute is one supplemental technical fact for a model.
type ArchitectureAttribute struct {
	Tag     string `json:"tag"`
	Display string `json:"display"`
	Value   string `json:"value"`
	Unit    string `json:"unit,omitempty"`
}

// Parameters is the list of documented controls and capabilities for a model.
type Parameters struct {
	Parameters   []Parameter  `json:"parameters"`
	Capabilities []Capability `json:"capabilities"`
}

// Parameter is one documented control parameter for a model, such as
// temperature, top_p, max_tokens, etc.
type Parameter struct {
	Tag      string `json:"tag"`
	Display  string `json:"display"`
	Category string `json:"category"`
}

// Capability is one documented capability for a model, such as tool use, image
// generation, etc.
type Capability struct {
	Tag     string `json:"tag"`
	Display string `json:"display"`
}

// Pricing is the list of published prices for a model.
type Pricing []Price

// Price is one published price for a model.
type Price struct {
	Tag      string `json:"tag"`
	Display  string `json:"display"`
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
	Unit     string `json:"unit"`
	Per      int64  `json:"per"`
}

// Limits is the list of published limits for a model.
type Limits []Limit

// Limit is one published limit for a model.
type Limit struct {
	Tag     string `json:"tag"`
	Display string `json:"display"`
	Maximum int64  `json:"maximum"`
	Unit    string `json:"unit"`
	Window  string `json:"window,omitempty"`
}
