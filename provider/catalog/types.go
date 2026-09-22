package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

//===============================================
// ProviderRawModel
//===============================================

// ProviderRawModel is the compressed (gzip) JSON data from a provider's
// catalog model.
type ProviderRawModel []byte

func NewProviderRawModel(data json.RawMessage) (*ProviderRawModel, error) {
	out := &ProviderRawModel{}
	return out, out.Encode(data)
}

// Decode decompresses the compressed (gzip) JSON data from a provider's catalog
// model.
func (c *ProviderRawModel) Decode() (json.RawMessage, error) {
	r, err := gzip.NewReader(bytes.NewReader(*c))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Encode compresses the JSON data from a provider's catalog model.
func (c *ProviderRawModel) Encode(data json.RawMessage) error {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return err
	}
	return w.Close()
}

//===============================================
// Model DTOs
//===============================================

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
