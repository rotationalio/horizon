package openai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/endeavor/pkg/horizon/catalog"
	"go.rtnl.ai/endeavor/pkg/horizon/client/auth/credtest"
	"go.rtnl.ai/endeavor/pkg/horizon/client/config"
	"go.rtnl.ai/endeavor/pkg/horizon/client/types"
	"go.rtnl.ai/endeavor/pkg/horizon/openai"
)

//=============================================================================
// Decode tests
//=============================================================================

// TestDecodeModelJSON verifies a single OpenAI model payload decodes into catalog.Model.
func TestDecodeModelJSON(t *testing.T) {
	model, err := openai.DecodeModelJSON([]byte(exampleModelJSON))
	require.NoError(t, err)
	assertExampleGPT4oModel(t, model)
}

// TestDecodeListJSON verifies an OpenAI list-models payload decodes every entry.
func TestDecodeListJSON(t *testing.T) {
	models, err := openai.DecodeListJSON([]byte(exampleListJSON))
	require.NoError(t, err)
	require.Len(t, models, 3)

	for _, model := range models {
		t.Run(model.Slug, func(t *testing.T) {
			assertExampleGPT4oModel(t, model)
		})
	}
}

//=============================================================================
// Catalog client tests
//=============================================================================

// TestFetch exercises CatalogClient.Fetch against a stub HTTP server.
func TestFetch(t *testing.T) {
	client := testCatalogClientWithServer(t)

	models, err := client.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 3)
	assertExampleGPT4oModel(t, models[0])
}

// TestRetrieve exercises CatalogClient.Retrieve against the OpenAI single-model endpoint.
func TestRetrieve(t *testing.T) {
	client := testCatalogClientWithServer(t)

	model, err := client.Retrieve(context.Background(), expectedGPT4oSlug)
	require.NoError(t, err)
	require.NotNil(t, model)
	assertExampleGPT4oModel(t, *model)
}

//=============================================================================
// Example JSON and Assertion Helpers
//=============================================================================

// Example captured from GET https://api.openai.com/v1/models/gpt-4o
const exampleModelJSON = `{
	"id": "gpt-4o",
	"object": "model",
	"created": 1715367049,
	"owned_by": "system"
}`

// exampleListJSON is a list-models response containing three copies of exampleModelJSON.
const exampleListJSON = `{
	"object": "list",
	"data": [` + exampleModelJSON + `,` + exampleModelJSON + `,` + exampleModelJSON + `]
}`

const (
	// expectedGPT4oSlug is the model ID from the example fixture.
	expectedGPT4oSlug = "gpt-4o"
	// expectedGPT4oAuthor is the owned_by field from the example fixture.
	expectedGPT4oAuthor = "system"
)

// expectedGPT4oPublished is the created timestamp from the example fixture.
var expectedGPT4oPublished = time.Unix(1715367049, 0)

// assertExampleGPT4oModel checks that model matches the example gpt-4o fixture.
func assertExampleGPT4oModel(t *testing.T, model catalog.Model) {
	t.Helper()

	require.Equal(t, types.ProviderTypeOpenAI, model.ProviderType)
	require.Equal(t, expectedGPT4oSlug, model.Slug)
	require.Equal(t, expectedGPT4oSlug, model.Name)
	require.Empty(t, model.Description)
	require.Equal(t, expectedGPT4oAuthor, model.Author)
	require.Equal(t, expectedGPT4oPublished, model.Published)
	require.Empty(t, model.License)
	require.Zero(t, model.InputModality)
	require.Zero(t, model.OutputModality)
	require.False(t, model.InputModality.IsText())
	require.False(t, model.OutputModality.IsText())
	require.Empty(t, model.Size)
	require.Nil(t, model.ContextSize)
	require.Nil(t, model.InputCost)
	require.Nil(t, model.OutputCost)
	require.Nil(t, model.TPMLimit)
	require.Nil(t, model.RPMLimit)
	require.Nil(t, model.IsModerated)
	require.Empty(t, model.Links)
	require.Empty(t, model.Architecture)
	require.Empty(t, model.Parameters.Parameters)
	require.Empty(t, model.Parameters.Capabilities)
	require.Empty(t, model.Pricing)
	require.Empty(t, model.Limits)
	require.False(t, model.EnergyUsage.Valid)
	require.Zero(t, model.APIType)
	require.Empty(t, model.Endpoint)
	require.Zero(t, model.AuthType)
	require.False(t, model.Restricted)
	require.Empty(t, model.Policies)
}

//=============================================================================
// Live tests and helpers
//=============================================================================

// TestFetchLive calls the real OpenAI models API. Skipped with -short or when
// OPENAI_API_KEY is unset.
func TestFetchLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OpenAI catalog test in short mode")
	}

	// TODO: no public endpoint for this API; need to add an API key for CI testing.
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := openai.NewCatalog(config.Provider{
		CatalogEndpoint: "https://api.openai.com/v1/models",
		Credentials:     credtest.APIKey(apiKey),
	})
	require.NoError(t, err)

	models, err := client.Fetch(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(models), 1)

	for _, model := range models {
		require.Equal(t, types.ProviderTypeOpenAI, model.ProviderType)
		require.NotEmpty(t, model.Slug)
		require.Equal(t, model.Slug, model.Name)
		require.NotEmpty(t, model.Author)
		require.False(t, model.Published.IsZero())
	}
}

// testCatalogClientWithServer returns a catalog client backed by an httptest server
// serving the example list and single-model fixtures.
func testCatalogClientWithServer(t *testing.T) *openai.CatalogClient {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, err := w.Write([]byte(exampleListJSON))
			require.NoError(t, err)
		case "/v1/models/gpt-4o":
			_, err := w.Write([]byte(exampleModelJSON))
			require.NoError(t, err)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	client, err := openai.NewCatalog(config.Provider{
		CatalogEndpoint: ts.URL + "/v1/models",
		Credentials:     credtest.APIKey("test-key"),
	})
	require.NoError(t, err)
	return client
}
