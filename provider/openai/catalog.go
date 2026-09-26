package openai

import (
	"context"
	"encoding/json"
	"time"

	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
)

//=============================================================================
// CatalogClient
//=============================================================================

// ConnectivityModel is the model used for provider connectivity checks.
const ConnectivityModel = "gpt-4o-mini"

// CatalogClient fetches model metadata from the OpenAI models API.
// See https://platform.openai.com/docs/api-reference/models
type CatalogClient struct {
	conf provider.Config
}

// NewCatalog creates an OpenAI catalog client.
func NewCatalog(conf provider.Config) (*CatalogClient, error) {
	return &CatalogClient{conf: conf}, nil
}

// Returns models from the OpenAI catalog.
func (c *CatalogClient) FetchCatalog(ctx context.Context) ([]catalog.Model, error) {
	body, err := http.GetSuffix(ctx, c.conf.CatalogEndpoint, "", c.conf.Credentials)
	if err != nil {
		return nil, err
	}

	models, err := DecodeListJSON(body)
	if err != nil {
		return nil, err
	}

	return models, nil
}

// Returns one model from the OpenAI catalog.
func (c *CatalogClient) RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error) {
	body, err := http.GetSuffix(ctx, c.conf.CatalogEndpoint, modelID, c.conf.Credentials)
	if err != nil {
		return nil, err
	}

	model, err := DecodeModelJSON(body)
	if err != nil {
		return nil, err
	}

	return &model, nil
}

//=============================================================================
// WireModel
//=============================================================================

// WireModel is the OpenAI models API object shape.
// See https://platform.openai.com/docs/api-reference/models/object
// TODO: figure out how to get more data for these models because the API is so limited.
type WireModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// WireList is the OpenAI list models response shape.
type WireList struct {
	Object string      `json:"object"`
	Data   []WireModel `json:"data"`
}

// ModelFromWire converts an OpenAI models API object into a catalog model.
func ModelFromWire(wire WireModel) catalog.Model {
	return catalog.Model{
		Name:      wire.ID,
		Slug:      wire.ID,
		Author:    wire.OwnedBy,
		Published: time.Unix(wire.Created, 0),
	}
}

// ModelsFromList converts an OpenAI list models response into catalog models.
func ModelsFromList(list WireList) []catalog.Model {
	out := make([]catalog.Model, 0, len(list.Data))
	for _, wire := range list.Data {
		out = append(out, ModelFromWire(wire))
	}
	return out
}

// DecodeModelJSON decodes a single OpenAI model JSON object.
func DecodeModelJSON(data []byte) (catalog.Model, error) {
	var wire WireModel
	if err := json.Unmarshal(data, &wire); err != nil {
		return catalog.Model{}, err
	}
	return ModelFromWire(wire), nil
}

// DecodeListJSON decodes an OpenAI list models JSON response.
func DecodeListJSON(data []byte) ([]catalog.Model, error) {
	var list WireList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return ModelsFromList(list), nil
}
