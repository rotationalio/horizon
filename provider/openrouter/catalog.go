package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
)

//=============================================================================
// CatalogClient
//=============================================================================

// ConnectivityModel is the model used for provider connectivity checks.
// TODO: use openrouter/free with openrouter/auto (cheap) as fallback once we
// can add provider-specific request extensions; gemini below is a cheap model
// we can use in the interim
const ConnectivityModel = "google/gemini-2.5-flash-lite"

// CatalogClient fetches model metadata from the OpenRouter models API.
// See https://openrouter.ai/docs/api/api-reference/models/get-models
type CatalogClient struct {
	conf provider.Config
}

// NewCatalog creates an OpenRouter catalog client.
func NewCatalog(conf provider.Config) (*CatalogClient, error) {
	return &CatalogClient{conf: conf}, nil
}

// Config returns the provider configuration for this catalog client.
func (c *CatalogClient) Config() provider.Config {
	if c == nil {
		return provider.Config{}
	}
	return c.conf
}

// Returns models from the OpenRouter catalog.
func (c *CatalogClient) FetchCatalog(ctx context.Context) ([]catalog.Model, error) {
	endpoint, err := catalogEndpointWithAllModalities(c.conf.CatalogEndpoint)
	if err != nil {
		return nil, err
	}

	body, err := http.GetSuffix(ctx, endpoint, "", c.conf.Credentials)
	if err != nil {
		return nil, err
	}

	models, err := c.DecodeListJSON(body)
	if err != nil {
		return nil, err
	}

	return models, nil
}

// Specify the modalities explicitly to ensure all models are returned.
// OpenRouter does not show video and speech generation models in the catalog by default.
func catalogEndpointWithAllModalities(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse catalog endpoint: %w", err)
	}

	query := parsed.Query()
	query.Set("output_modalities", "all")
	query.Set("input_modalities", "all")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// Returns one model from the OpenRouter catalog.
func (c *CatalogClient) RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error) {
	endpoint := strings.TrimSuffix(c.conf.CatalogEndpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/models") + "/model/" + strings.TrimPrefix(modelID, "/")

	body, err := http.Get(ctx, endpoint, c.conf.Credentials)
	if err != nil {
		return nil, err
	}

	var wire struct {
		Data WireModel `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}

	model, err := c.ModelFromWire(wire.Data)
	if err != nil {
		return nil, err
	}

	return &model, nil
}

//=============================================================================
// Wire types
//=============================================================================

// WireModel is the OpenRouter models API object shape.
type WireModel struct {
	ID                  string                `json:"id"`
	CanonicalSlug       string                `json:"canonical_slug"`
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	Created             int64                 `json:"created"`
	ContextLength       *int64                `json:"context_length"`
	HuggingFaceID       *string               `json:"hugging_face_id"`
	Architecture        WireArchitecture      `json:"architecture"`
	Pricing             WirePricing           `json:"pricing"`
	TopProvider         WireTopProvider       `json:"top_provider"`
	SupportedParameters []string              `json:"supported_parameters"`
	Links               WireLinks             `json:"links"`
	PerRequestLimits    *WirePerRequestLimits `json:"per_request_limits"`
}

// WireArchitecture is the OpenRouter model architecture object.
type WireArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Modality         *string  `json:"modality"`
	InstructType     *string  `json:"instruct_type"`
	Tokenizer        string   `json:"tokenizer"`
}

// WirePricing is the OpenRouter public pricing object.
type WirePricing struct {
	Prompt            string `json:"prompt"`
	Completion        string `json:"completion"`
	Image             string `json:"image"`
	Request           string `json:"request"`
	WebSearch         string `json:"web_search"`
	InternalReasoning string `json:"internal_reasoning"`
}

// WireTopProvider is the OpenRouter top provider metadata object.
type WireTopProvider struct {
	IsModerated         bool   `json:"is_moderated"`
	ContextLength       *int64 `json:"context_length"`
	MaxCompletionTokens *int64 `json:"max_completion_tokens"`
}

// WireLinks is the OpenRouter related-links object.
type WireLinks struct {
	Details string `json:"details"`
}

// WirePerRequestLimits is the OpenRouter per-request token limits object.
type WirePerRequestLimits struct {
	PromptTokens     float64 `json:"prompt_tokens"`
	CompletionTokens float64 `json:"completion_tokens"`
}

// WireList is the OpenRouter list models response shape.
type WireList struct {
	Data []WireModel `json:"data"`
}

//=============================================================================
// Decode and conversion
//=============================================================================

// ModelFromWire converts an OpenRouter models API object into a catalog model.
func (c *CatalogClient) ModelFromWire(wire WireModel) (catalog.Model, error) {
	slug := wire.ID
	if slug == "" {
		slug = wire.CanonicalSlug
	}

	name := wire.Name
	if name == "" {
		name = slug
	}

	out := catalog.Model{
		Name:           name,
		Slug:           slug,
		Description:    wire.Description,
		Author:         authorFromSlug(slug),
		Published:      time.Unix(wire.Created, 0),
		InputModality:  wireModalities(wire.Architecture.InputModalities),
		OutputModality: wireModalities(wire.Architecture.OutputModalities),
		ContextSize:    contextSize(wire.ContextLength, wire.TopProvider.ContextLength),
		Architecture:   architectureFromWire(wire.Architecture),
		Parameters:     parametersFromWire(wire.SupportedParameters),
		Capabilities:   wireCapabilities(wire.SupportedParameters),
		Pricing:        pricingFromWire(wire.Pricing),
		Limits:         limitsFromWire(wire.PerRequestLimits, wire.TopProvider),
		Links:          c.linksFromWire(wire.Links, wire.HuggingFaceID),
		IsModerated:    &wire.TopProvider.IsModerated,
	}

	if inputCost, err := parsePrice(wire.Pricing.Prompt); err != nil {
		return catalog.Model{}, fmt.Errorf("parse prompt pricing: %w", err)
	} else if inputCost != nil {
		out.InputCost = inputCost
	}

	if outputCost, err := parsePrice(wire.Pricing.Completion); err != nil {
		return catalog.Model{}, fmt.Errorf("parse completion pricing: %w", err)
	} else if outputCost != nil {
		out.OutputCost = outputCost
	}

	// Include tool turns parameter if the model supports tool calling.
	if out.Capabilities&catalog.Tools != 0 {
		out.Parameters.Parameters = append(out.Parameters.Parameters, catalog.Parameter{
			Tag:      params.MaxToolTurns,
			Display:  "Max Tool Turns",
			Category: "parameter",
		})
	}

	return out, nil
}

// ModelsFromList converts an OpenRouter list models response into catalog models.
func (c *CatalogClient) ModelsFromList(list WireList) ([]catalog.Model, error) {
	out := make([]catalog.Model, 0, len(list.Data))
	for _, wire := range list.Data {
		model, err := c.ModelFromWire(wire)
		if err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, nil
}

// DecodeModelJSON decodes a single OpenRouter model JSON object.
func (c *CatalogClient) DecodeModelJSON(data []byte) (catalog.Model, error) {
	var wire WireModel
	if err := json.Unmarshal(data, &wire); err != nil {
		return catalog.Model{}, err
	}
	return c.ModelFromWire(wire)
}

// DecodeListJSON decodes an OpenRouter list models JSON response.
func (c *CatalogClient) DecodeListJSON(data []byte) ([]catalog.Model, error) {
	var list WireList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return c.ModelsFromList(list)
}

//=============================================================================
// Catalog URL helpers
//=============================================================================

func (c *CatalogClient) linksFromWire(links WireLinks, huggingFaceID *string) catalog.Links {
	out := make(catalog.Links, 0, 2)

	if links.Details != "" {
		out = append(out, catalog.Link{
			Tag:     "details",
			Display: "Model Details",
			URL:     c.absoluteCatalogURL(links.Details),
		})
	}

	if huggingFaceID != nil && *huggingFaceID != "" {
		out = append(out, catalog.Link{
			Tag:     "hugging_face",
			Display: "Hugging Face",
			URL:     "https://huggingface.co/" + strings.TrimPrefix(*huggingFaceID, "/"),
		})
	}

	return out
}

func (c *CatalogClient) absoluteCatalogURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(c.catalogOrigin(), "/") + "/" + strings.TrimPrefix(path, "/")
}

func (c *CatalogClient) catalogOrigin() string {
	u := c.conf.CatalogURL()
	if u == nil || u.Host == "" {
		return ""
	}

	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host
}

//=============================================================================
// Slug helpers
//=============================================================================

func authorFromSlug(slug string) string {
	author, _, ok := strings.Cut(slug, "/")
	if !ok {
		return ""
	}
	return author
}

//=============================================================================
// Modality conversion
//=============================================================================

func wireModalities(values []string) modality.Modality {
	var out modality.Modality
	for _, value := range values {
		if m, ok := parseWireModality(value); ok {
			out |= m
		}
	}
	return out
}

func parseWireModality(value string) (modality.Modality, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "file":
		return modality.Document, true
	case "embeddings", "rerank":
		// TODO: what modalities are these?
		return 0, false
	case "speech", "transcription":
		return modality.Audio, true
	default:
		m, err := modality.Parse(value)
		return m, err == nil
	}
}

//=============================================================================
// Capabilities conversion
//=============================================================================

func wireCapabilities(values []string) catalog.ModelCapability {
	var out catalog.ModelCapability
	for _, value := range values {
		if c, ok := parseWireCapability(value); ok {
			out |= c
		}
	}
	return out
}

func parseWireCapability(value string) (catalog.ModelCapability, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "web_search_options":
		return catalog.WebSearch, true
	default:
		c, err := catalog.Parse(value)
		return c, err == nil
	}
}

//=============================================================================
// Numeric conversion
//=============================================================================

func contextSize(primary, fallback *int64) *int32 {
	if size := int64PtrToInt32(primary); size != nil {
		return size
	}
	return int64PtrToInt32(fallback)
}

func int64PtrToInt32(v *int64) *int32 {
	if v == nil {
		return nil
	}
	out := int32(*v)
	return &out
}

//=============================================================================
// Architecture conversion
//=============================================================================

func architectureFromWire(wire WireArchitecture) catalog.Architecture {
	out := make(catalog.Architecture, 0, 4)

	if wire.Tokenizer != "" {
		out = append(out, catalog.ArchitectureAttribute{
			Tag:     "tokenizer",
			Display: "Tokenizer",
			Value:   wire.Tokenizer,
		})
	}
	if wire.InstructType != nil && *wire.InstructType != "" {
		out = append(out, catalog.ArchitectureAttribute{
			Tag:     "instruct_type",
			Display: "Instruction Format",
			Value:   *wire.InstructType,
		})
	}
	if wire.Modality != nil && *wire.Modality != "" {
		out = append(out, catalog.ArchitectureAttribute{
			Tag:     "modality",
			Display: "Modality",
			Value:   *wire.Modality,
		})
	}

	return out
}

//=============================================================================
// Parameters conversion
//=============================================================================

func parametersFromWire(values []string) catalog.Parameters {
	out := catalog.Parameters{
		Parameters: make([]catalog.Parameter, 0, len(values)),
	}
	for _, value := range values {
		out.Parameters = append(out.Parameters, catalog.Parameter{
			Tag:      value,
			Display:  humanizeTag(value),
			Category: "parameter",
		})
	}
	return out
}

func humanizeTag(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

//=============================================================================
// Pricing conversion
//=============================================================================

func pricingFromWire(wire WirePricing) catalog.Pricing {
	fields := []struct {
		tag     string
		display string
		amount  string
		unit    string
	}{
		{tag: "prompt", display: "Prompt", amount: wire.Prompt, unit: "token"},
		{tag: "completion", display: "Completion", amount: wire.Completion, unit: "token"},
		{tag: "image", display: "Image", amount: wire.Image, unit: "image"},
		{tag: "request", display: "Request", amount: wire.Request, unit: "request"},
		{tag: "web_search", display: "Web Search", amount: wire.WebSearch, unit: "search"},
		{tag: "internal_reasoning", display: "Internal Reasoning", amount: wire.InternalReasoning, unit: "token"},
	}

	out := make(catalog.Pricing, 0, len(fields))
	for _, field := range fields {
		amount := strings.TrimSpace(field.amount)
		if amount == "" || amount == "0" {
			continue
		}
		out = append(out, catalog.Price{
			Tag:      field.tag,
			Display:  field.display,
			Currency: "USD",
			Amount:   field.amount,
			Unit:     field.unit,
			Per:      1,
		})
	}
	return out
}

func parsePrice(amount string) (*float64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}

	value, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

//=============================================================================
// Limits conversion
//=============================================================================

func limitsFromWire(perRequest *WirePerRequestLimits, topProvider WireTopProvider) catalog.Limits {
	out := make(catalog.Limits, 0, 3)

	if perRequest != nil {
		if perRequest.PromptTokens > 0 {
			out = append(out, catalog.Limit{
				Tag:     "prompt_tokens",
				Display: "Prompt Tokens",
				Maximum: int64(perRequest.PromptTokens),
				Unit:    "tokens",
				Window:  "request",
			})
		}
		if perRequest.CompletionTokens > 0 {
			out = append(out, catalog.Limit{
				Tag:     "completion_tokens",
				Display: "Completion Tokens",
				Maximum: int64(perRequest.CompletionTokens),
				Unit:    "tokens",
				Window:  "request",
			})
		}
	}

	if topProvider.MaxCompletionTokens != nil && *topProvider.MaxCompletionTokens > 0 {
		out = append(out, catalog.Limit{
			Tag:     "max_completion_tokens",
			Display: "Max Completion Tokens",
			Maximum: *topProvider.MaxCompletionTokens,
			Unit:    "tokens",
		})
	}

	return out
}
