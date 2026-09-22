package catalog

import (
	"time"

	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/catalog/governance"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/tidal/fields"
)

// Model is the canonical catalog representation of a provider model. It contains
// display metadata normalized from provider wire formats and is the shape
// written to JSONB catalog columns in persistence adapters.
type Model struct {
	ProviderType types.ProviderType `json:"provider_type"`

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

	Links        Links                        `json:"links,omitzero"`
	Architecture Architecture                 `json:"architecture,omitzero"`
	Parameters   Parameters                   `json:"parameters,omitzero"`
	Capabilities capabilities.ModelCapability `json:"capabilities,omitzero"`
	Pricing      Pricing                      `json:"pricing,omitzero"`
	Limits       Limits                       `json:"limits,omitzero"`

	// TODO: figure out a common format for energy usage
	EnergyUsage fields.NullJSONB `json:"energy_usage,omitzero"`

	// Model-specific API, endpoint, and auth overrides. Not commonly used.

	APIType  types.APIType `json:"api_type,omitzero"`
	Endpoint string        `json:"endpoint,omitempty"`
	AuthType auth.Type     `json:"auth_type,omitzero"`

	// If true, the model is restricted by the included governance policies.

	Restricted bool                `json:"restricted,omitempty"`
	Policies   []governance.Policy `json:"policies,omitzero"`
}

//=============================================================================
// Governance Subject Implementation
//=============================================================================

var _ governance.Subject = (*Model)(nil)

// LicenseName returns the license that governs use of the model.
func (m *Model) LicenseName() string {
	if m == nil {
		return ""
	}
	return m.License
}

// ContextSizeValue returns the maximum context size in tokens.
func (m *Model) ContextSizeValue() (int32, bool) {
	if m == nil || m.ContextSize == nil {
		return 0, false
	}
	return *m.ContextSize, true
}

// InputCostValue returns the estimated USD cost per input token.
func (m *Model) InputCostValue() (float64, bool) {
	if m == nil || m.InputCost == nil {
		return 0, false
	}
	return *m.InputCost, true
}
