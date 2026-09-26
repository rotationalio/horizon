package openrouter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/openrouter"
)

//=============================================================================
// Decode tests
//=============================================================================

// Verifies a single OpenRouter model payload decodes into catalog.Model.
func TestDecodeModelJSON(t *testing.T) {
	client := testCatalogClient(t)

	model, err := client.DecodeModelJSON([]byte(exampleModelJSON))
	require.NoError(t, err)
	assertExampleGPT4Model(t, model)
}

// Verifies an OpenRouter list-models payload decodes every entry.
func TestDecodeListJSON(t *testing.T) {
	client := testCatalogClient(t)

	models, err := client.DecodeListJSON([]byte(exampleListJSON))
	require.NoError(t, err)
	require.Len(t, models, 3)

	for _, model := range models {
		t.Run(model.Slug, func(t *testing.T) {
			assertExampleGPT4Model(t, model)
		})
	}
}

//=============================================================================
// Catalog client tests
//=============================================================================

// Exercises catalog fetching against a stub HTTP server.
func TestFetch(t *testing.T) {
	client, origin := testCatalogClientWithServer(t)

	models, err := client.FetchCatalog(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 3)
	assertExampleGPT4Model(t, models[0], origin)
}

// Exercises catalog retrieval against the OpenRouter single-model endpoint.
func TestRetrieve(t *testing.T) {
	client, origin := testCatalogClientWithServer(t)

	model, err := client.RetrieveModel(context.Background(), "openai/gpt-4")
	require.NoError(t, err)
	require.NotNil(t, model)
	assertExampleGPT4Model(t, *model, origin)
}

// Calls the real OpenRouter models API; skips with -short.
func TestFetchLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OpenRouter catalog test in short mode")
	}

	client := testCatalogClient(t)

	models, err := client.FetchCatalog(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(models), 1)

	for _, model := range models {
		require.NotEmpty(t, model.Slug)
		require.NotEmpty(t, model.Name)
		require.Contains(t, model.Slug, "/")
		require.False(t, model.Published.IsZero())
	}
}

//=============================================================================
// Fixtures
//=============================================================================

// Example captured from GET https://openrouter.ai/api/v1/models?q=openai/gpt-4.
//
// Additional wire fields that were null or omitted in that response were populated
// below so decoding exercises every field our WireModel maps into catalog.Model.
// API-only fields (benchmarks, default_parameters, etc.) are retained to ensure
// they do not break JSON unmarshaling.
const exampleModelJSON = `{
	"id": "openai/gpt-4",
	"canonical_slug": "openai/gpt-4",
	"hugging_face_id": "openai/gpt-4",
	"name": "OpenAI: GPT-4",
	"created": 1685232000,
	"description": "OpenAI's flagship model, GPT-4 is a large-scale multimodal language model capable of solving difficult problems with greater accuracy than previous models due to its broader general knowledge and advanced reasoning...",
	"context_length": 8191,
	"architecture": {
		"modality": "text+image->text",
		"input_modalities": [
			"text",
			"image",
			"file"
		],
		"output_modalities": [
			"text"
		],
		"tokenizer": "GPT",
		"instruct_type": "chatml"
	},
	"pricing": {
		"prompt": "0.00003",
		"completion": "0.00006",
		"image": "0.00001",
		"request": "0.000005",
		"web_search": "0.00002",
		"internal_reasoning": "0.00001"
	},
	"top_provider": {
		"context_length": 8191,
		"max_completion_tokens": 4096,
		"is_moderated": false
	},
	"per_request_limits": {
		"prompt_tokens": 8191,
		"completion_tokens": 4096
	},
	"supported_parameters": [
		"frequency_penalty",
		"logit_bias",
		"logprobs",
		"max_completion_tokens",
		"max_tokens",
		"presence_penalty",
		"response_format",
		"seed",
		"stop",
		"structured_outputs",
		"temperature",
		"tool_choice",
		"tools",
		"top_logprobs",
		"top_p"
	],
	"default_parameters": {
		"temperature": 0.7,
		"top_p": 1
	},
	"supported_voices": null,
	"knowledge_cutoff": "2021-09-30",
	"expiration_date": null,
	"links": {
		"details": "/api/v1/models/openai/gpt-4/endpoints"
	},
	"benchmarks": {
		"design_arena": [],
		"artificial_analysis": {
			"intelligence_index": null,
			"coding_index": 13.1,
			"agentic_index": null
		}
	}
}`

// exampleListJSON is a list-models response containing three copies of exampleModelJSON.
const exampleListJSON = `{"data":[` + exampleModelJSON + `,` + exampleModelJSON + `,` + exampleModelJSON + `]}`

// expectedGPT4Desc is the description string asserted for the example GPT-4 model.
const expectedGPT4Desc = "OpenAI's flagship model, GPT-4 is a large-scale multimodal language model capable of solving difficult problems with greater accuracy than previous models due to its broader general knowledge and advanced reasoning..."

// expectedGPT4Params is the supported_parameters list from the example model fixture.
var expectedGPT4Params = []string{
	"frequency_penalty",
	"logit_bias",
	"logprobs",
	"max_completion_tokens",
	"max_tokens",
	"presence_penalty",
	"response_format",
	"seed",
	"stop",
	"structured_outputs",
	"temperature",
	"tool_choice",
	"tools",
	"top_logprobs",
	"top_p",
	"max_tool_turns",
}

// assertExampleGPT4Model checks that model matches the example GPT-4 fixture.
// An optional catalogOrigin overrides the default OpenRouter host used for relative link URLs.
func assertExampleGPT4Model(t *testing.T, model catalog.Model, catalogOrigin ...string) {
	t.Helper()

	origin := "https://openrouter.ai"
	if len(catalogOrigin) > 0 {
		origin = strings.TrimRight(catalogOrigin[0], "/")
	}

	require.Equal(t, "openai/gpt-4", model.Slug)
	require.Equal(t, "OpenAI: GPT-4", model.Name)
	require.Equal(t, expectedGPT4Desc, model.Description)
	require.Equal(t, "openai", model.Author)
	require.Equal(t, time.Unix(1685232000, 0), model.Published)
	require.Empty(t, model.License)
	require.Equal(t, modality.Text|modality.Image|modality.Document, model.InputModality)
	require.Equal(t, modality.Text, model.OutputModality)
	require.Empty(t, model.Size)

	require.NotNil(t, model.ContextSize)
	require.Equal(t, int32(8191), *model.ContextSize)

	require.NotNil(t, model.InputCost)
	require.InDelta(t, 0.00003, *model.InputCost, 0)
	require.NotNil(t, model.OutputCost)
	require.InDelta(t, 0.00006, *model.OutputCost, 0)
	require.Nil(t, model.TPMLimit)
	require.Nil(t, model.RPMLimit)

	require.NotNil(t, model.IsModerated)
	require.False(t, *model.IsModerated)

	require.Len(t, model.Architecture, 3)
	require.Equal(t, catalog.ArchitectureAttribute{
		Tag:     "tokenizer",
		Display: "Tokenizer",
		Value:   "GPT",
	}, model.Architecture[0])
	require.Equal(t, catalog.ArchitectureAttribute{
		Tag:     "instruct_type",
		Display: "Instruction Format",
		Value:   "chatml",
	}, model.Architecture[1])
	require.Equal(t, catalog.ArchitectureAttribute{
		Tag:     "modality",
		Display: "Modality",
		Value:   "text+image->text",
	}, model.Architecture[2])

	require.Empty(t, model.Parameters.Capabilities)
	require.Len(t, model.Parameters.Parameters, len(expectedGPT4Params))
	for i, tag := range expectedGPT4Params {
		require.Equal(t, tag, model.Parameters.Parameters[i].Tag)
		require.Equal(t, "parameter", model.Parameters.Parameters[i].Category)
		require.NotEmpty(t, model.Parameters.Parameters[i].Display)
	}

	require.Len(t, model.Pricing, 6)
	require.Equal(t, catalog.Price{
		Tag: "prompt", Display: "Prompt", Currency: "USD",
		Amount: "0.00003", Unit: "token", Per: 1,
	}, model.Pricing[0])
	require.Equal(t, catalog.Price{
		Tag: "completion", Display: "Completion", Currency: "USD",
		Amount: "0.00006", Unit: "token", Per: 1,
	}, model.Pricing[1])
	require.Equal(t, catalog.Price{
		Tag: "image", Display: "Image", Currency: "USD",
		Amount: "0.00001", Unit: "image", Per: 1,
	}, model.Pricing[2])
	require.Equal(t, catalog.Price{
		Tag: "request", Display: "Request", Currency: "USD",
		Amount: "0.000005", Unit: "request", Per: 1,
	}, model.Pricing[3])
	require.Equal(t, catalog.Price{
		Tag: "web_search", Display: "Web Search", Currency: "USD",
		Amount: "0.00002", Unit: "search", Per: 1,
	}, model.Pricing[4])
	require.Equal(t, catalog.Price{
		Tag: "internal_reasoning", Display: "Internal Reasoning", Currency: "USD",
		Amount: "0.00001", Unit: "token", Per: 1,
	}, model.Pricing[5])

	require.Len(t, model.Limits, 3)
	require.Equal(t, catalog.Limit{
		Tag:     "prompt_tokens",
		Display: "Prompt Tokens",
		Maximum: 8191,
		Unit:    "tokens",
		Window:  "request",
	}, model.Limits[0])
	require.Equal(t, catalog.Limit{
		Tag:     "completion_tokens",
		Display: "Completion Tokens",
		Maximum: 4096,
		Unit:    "tokens",
		Window:  "request",
	}, model.Limits[1])
	require.Equal(t, catalog.Limit{
		Tag:     "max_completion_tokens",
		Display: "Max Completion Tokens",
		Maximum: 4096,
		Unit:    "tokens",
	}, model.Limits[2])

	require.Len(t, model.Links, 2)
	require.Equal(t, catalog.Link{
		Tag:     "details",
		Display: "Model Details",
		URL:     origin + "/api/v1/models/openai/gpt-4/endpoints",
	}, model.Links[0])
	require.Equal(t, catalog.Link{
		Tag:     "hugging_face",
		Display: "Hugging Face",
		URL:     "https://huggingface.co/openai/gpt-4",
	}, model.Links[1])

	require.False(t, model.EnergyUsage.Valid)
	require.Empty(t, model.Endpoint)
	require.Zero(t, model.AuthType)
}

// testCatalogClient returns a catalog client pointed at the production OpenRouter endpoint.
func testCatalogClient(t *testing.T) *openrouter.CatalogClient {
	t.Helper()

	client, err := openrouter.NewCatalog(provider.Config{
		CatalogEndpoint: "https://openrouter.ai/api/v1/models",
	})
	require.NoError(t, err)
	return client
}

// testCatalogClientWithServer returns a catalog client backed by an httptest server
// serving the example list and single-model fixtures. The server origin is returned
// so link assertions can resolve relative catalog URLs correctly.
func testCatalogClientWithServer(t *testing.T) (*openrouter.CatalogClient, string) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/models":
			_, err := w.Write([]byte(exampleListJSON))
			require.NoError(t, err)
		case "/api/v1/model/openai/gpt-4":
			_, err := w.Write([]byte(`{"data":` + exampleModelJSON + `}`))
			require.NoError(t, err)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	client, err := openrouter.NewCatalog(provider.Config{
		CatalogEndpoint: ts.URL + "/api/v1/models",
		Credentials:     auth.NewAPIKey("test-key"),
	})
	require.NoError(t, err)
	return client, ts.URL
}
