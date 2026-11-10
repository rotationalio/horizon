package openrouter_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/openrouter"
)

const jevAnswers = `{
	"id": "gen-dec-1",
	"model": "typesafe/jev-1.14-20260917",
	"provider": "TypeSafe",
	"answers": {
		"is_bug": {"type": "noul", "noul": 0.96},
		"team": {"type": "choice", "choice": "payments", "confidence": 0.67,
			"probabilities": {"payments": 0.78, "frontend": 0.22}}
	},
	"usage": {"input_tokens": 476, "output_tokens": 70, "cost": 0.000019992}
}`

const clairicAnswers = `{
	"id": "gen-dec-2",
	"model": "cloudflare/clairic",
	"provider": "Cloudflare",
	"answers": {
		"customer_name": {"type": "text", "value": "Dana Whitfield", "span": [30, 44], "p_null": 0.001, "confidence": 0.99}
	},
	"truncated": true,
	"usage": {"input_tokens": 120, "output_tokens": 0, "cost": 0.000012}
}`

// A catalog that serves fixed models and counts lookups.
type fakeCatalog struct {
	mu     sync.Mutex
	models map[string]*catalog.Model
	calls  int
}

func (f *fakeCatalog) RetrieveModel(_ context.Context, id string) (*catalog.Model, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	model, ok := f.models[id]
	if !ok {
		return nil, errors.ErrProviderNotFound
	}
	return model, nil
}

func newCatalog() *fakeCatalog {
	return &fakeCatalog{models: map[string]*catalog.Model{
		"typesafe/jev-1.14":  {Slug: "typesafe/jev-1.14", OutputModality: modality.Decisions, Endpoints: []string{"/api/alpha/decisions"}},
		"cloudflare/clairic": {Slug: "cloudflare/clairic", OutputModality: modality.Decisions, Endpoints: []string{"/api/alpha/extractions"}},
		"example/chat":       {Slug: "example/chat", Endpoints: []string{"/api/v1/chat/completions", "/api/alpha/extractions"}},
		"example/none":       {Slug: "example/none"},
	}}
}

type captured struct {
	mu     sync.Mutex
	path   string
	method string
	auth   string
	body   map[string]any
}

// Serves canned responses per route and records the last request.
func newServer(t *testing.T) (*httptest.Server, *captured) {
	t.Helper()

	got := &captured{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		got.mu.Lock()
		got.path, got.method, got.auth = r.URL.Path, r.Method, r.Header.Get("Authorization")
		got.body = nil
		require.NoError(t, json.Unmarshal(data, &got.body))
		got.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/alpha/decisions":
			_, _ = w.Write([]byte(jevAnswers))
		case "/api/alpha/extractions":
			_, _ = w.Write([]byte(clairicAnswers))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, got
}

func newClient(t *testing.T, ts *httptest.Server, models *fakeCatalog) *openrouter.DecisionsClient {
	t.Helper()

	client, err := openrouter.NewDecisions(provider.Config{
		InferenceEndpoint: ts.URL + "/api/v1",
		Credentials:       auth.NewAPIKey("test-key"),
	}, provider.ResolveOptions(), models)
	require.NoError(t, err)
	return client
}

func userInput(content string) prompts.Prompts {
	return prompts.Prompts{{Role: prompts.RoleUser, Content: content}}
}

func TestDecisionsClient_DecisionsRoute(t *testing.T) {
	ts, got := newServer(t)
	client := newClient(t, ts, newCatalog())

	rep, err := client.Generate(context.Background(), &provider.Request{
		Model: "typesafe/jev-1.14",
		Input: userInput("Checkout is blank after I click Pay."),
		Decisions: &provider.Decisions{
			Questions: map[string]provider.Question{
				"is_bug": {Type: provider.QuestionNoul, Instructions: "Is this a defect?"},
				"team": {Type: provider.QuestionChoice, Instructions: "Which team?",
					Criteria: map[string]string{"payments": "Billing", "frontend": "UI"}},
			},
			// Fields are ignored on the decisions route.
			Fields: map[string]provider.Field{"ignored": {Type: provider.FieldText}},
		},
	})
	require.NoError(t, err)

	// The request goes to the alpha decisions route, beside /api/v1, with questions.
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/api/alpha/decisions", got.path)
	require.Equal(t, "Bearer test-key", got.auth)
	require.Equal(t, "typesafe/jev-1.14", got.body["model"])
	require.Equal(t, "Checkout is blank after I click Pay.", got.body["state"])
	require.Contains(t, got.body, "questions")
	require.NotContains(t, got.body, "fields")
	require.NotContains(t, got.body, "reference_date")

	questions := got.body["questions"].(map[string]any)
	require.Equal(t, "noul", questions["is_bug"].(map[string]any)["type"])

	// The response carries typed answers, usage, and a JSON copy in the output.
	require.Equal(t, "gen-dec-1", rep.ID)
	require.Equal(t, "typesafe/jev-1.14", rep.Model)
	require.Equal(t, "typesafe/jev-1.14-20260917", rep.Meta["served_model"])
	require.Len(t, rep.Answers, 2)
	require.Equal(t, "noul", rep.Answers["is_bug"].Type)
	require.Equal(t, "choice", rep.Answers["team"].Type)
	require.JSONEq(t, `{"type": "noul", "noul": 0.96}`, string(rep.Answers["is_bug"].Raw))
	require.Equal(t, int64(476), rep.Usage.InputTokens)
	require.Equal(t, int64(70), rep.Usage.OutputTokens)
	require.Equal(t, int64(546), rep.Usage.TotalTokens)
	require.InDelta(t, 0.000019992, rep.Usage.APICost, 1e-12)
	require.Equal(t, "TypeSafe", rep.Meta["provider"])
	require.Equal(t, "/api/alpha/decisions", rep.Meta["route"])
	require.NotContains(t, rep.Meta, "truncated")

	require.Len(t, rep.Output, 1)
	require.Equal(t, prompts.RoleAssistant, rep.Output[0].Role)
	require.JSONEq(t, `{
		"is_bug": {"type": "noul", "noul": 0.96},
		"team": {"type": "choice", "choice": "payments", "confidence": 0.67,
			"probabilities": {"payments": 0.78, "frontend": 0.22}}
	}`, rep.Output[0].Content)
}

// Clairic is the edge case: a "decisions" modality model served on the
// extractions route, so the same request type must switch to fields.
func TestDecisionsClient_ExtractionsRoute_Clairic(t *testing.T) {
	ts, got := newServer(t)
	client := newClient(t, ts, newCatalog())

	rep, err := client.Generate(context.Background(), &provider.Request{
		Model: "cloudflare/clairic",
		Input: userInput("Hi, this is Dana Whitfield from Larkspur Labs."),
		Decisions: &provider.Decisions{
			Questions:     map[string]provider.Question{"ignored": {Type: provider.QuestionNoul}},
			Fields:        map[string]provider.Field{"customer_name": {Type: provider.FieldText, Instructions: "The name of the person writing"}},
			ReferenceDate: "2026-11-09",
		},
	})
	require.NoError(t, err)

	require.Equal(t, "/api/alpha/extractions", got.path)
	require.Equal(t, "cloudflare/clairic", got.body["model"])
	require.Equal(t, "Hi, this is Dana Whitfield from Larkspur Labs.", got.body["state"])
	require.Equal(t, "2026-11-09", got.body["reference_date"])
	require.Contains(t, got.body, "fields")
	require.NotContains(t, got.body, "questions")

	field := got.body["fields"].(map[string]any)["customer_name"].(map[string]any)
	require.Equal(t, "text", field["type"])
	require.Equal(t, "The name of the person writing", field["instructions"])

	require.Len(t, rep.Answers, 1)
	require.Equal(t, "text", rep.Answers["customer_name"].Type)
	require.Equal(t, "/api/alpha/extractions", rep.Meta["route"])
	require.Equal(t, true, rep.Meta["truncated"])
}

func TestDecisionsClient_SpecMustMatchRoute(t *testing.T) {
	ts, _ := newServer(t)
	client := newClient(t, ts, newCatalog())

	t.Run("DecisionsModelNeedsQuestions", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{
			Model:     "typesafe/jev-1.14",
			Input:     userInput("text"),
			Decisions: &provider.Decisions{Fields: map[string]provider.Field{"a": {Type: provider.FieldText}}},
		})
		require.ErrorIs(t, err, errors.ErrDecisionSpecRequired)
		require.ErrorContains(t, err, "needs questions")
	})

	t.Run("ExtractionsModelNeedsFields", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{
			Model:     "cloudflare/clairic",
			Input:     userInput("text"),
			Decisions: &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}},
		})
		require.ErrorIs(t, err, errors.ErrDecisionSpecRequired)
		require.ErrorContains(t, err, "needs fields")
	})

	t.Run("MissingSpec", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{Model: "typesafe/jev-1.14", Input: userInput("text")})
		require.ErrorIs(t, err, errors.ErrDecisionSpecRequired)
	})
}

func TestDecisionsClient_RouteSelection(t *testing.T) {
	ts, _ := newServer(t)
	client := newClient(t, ts, newCatalog())
	spec := &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}}

	t.Run("FirstEndpointIsNotATypedRoute", func(t *testing.T) {
		// Index 0 decides the route, even though extractions appears later.
		_, err := client.Generate(context.Background(), &provider.Request{Model: "example/chat", Input: userInput("text"), Decisions: spec})
		require.ErrorIs(t, err, errors.ErrUnsupportedModelEndpoint)
		require.ErrorContains(t, err, "/api/v1/chat/completions")
	})

	t.Run("NoEndpoints", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{Model: "example/none", Input: userInput("text"), Decisions: spec})
		require.ErrorIs(t, err, errors.ErrNoModelEndpoints)
	})

	t.Run("UnknownModel", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{Model: "example/missing", Input: userInput("text"), Decisions: spec})
		require.ErrorIs(t, err, errors.ErrProviderNotFound)
	})
}

func TestDecisionsClient_RouteIsCached(t *testing.T) {
	ts, _ := newServer(t)
	models := newCatalog()
	client := newClient(t, ts, models)

	req := &provider.Request{
		Model:     "typesafe/jev-1.14",
		Input:     userInput("text"),
		Decisions: &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}},
	}
	for range 3 {
		_, err := client.Generate(context.Background(), req)
		require.NoError(t, err)
	}
	require.Equal(t, 1, models.calls, "the catalog should be asked once for repeated calls")
}

func TestDecisionsClient_RejectsUnsupportedInput(t *testing.T) {
	ts, _ := newServer(t)
	client := newClient(t, ts, newCatalog())
	spec := &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}}

	cases := map[string]*provider.Request{
		"SystemPrompt": {Model: "typesafe/jev-1.14", Decisions: spec, Input: prompts.Prompts{
			{Role: prompts.RoleSystem, Content: "be helpful"},
			{Role: prompts.RoleUser, Content: "text"},
		}},
		"Attachments": {Model: "typesafe/jev-1.14", Decisions: spec, Input: userInput("text"),
			Attachments: attachments.Attachments{{}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Generate(context.Background(), req)
			require.ErrorIs(t, err, errors.ErrUnsupportedDecisionInput)
		})
	}

	t.Run("EmptyState", func(t *testing.T) {
		_, err := client.Generate(context.Background(), &provider.Request{Model: "typesafe/jev-1.14", Decisions: spec, Input: userInput("  ")})
		require.ErrorIs(t, err, errors.ErrMissingInput)
	})

	t.Run("MultipleUserPromptsAreJoined", func(t *testing.T) {
		ts, got := newServer(t)
		client := newClient(t, ts, newCatalog())
		_, err := client.Generate(context.Background(), &provider.Request{
			Model: "typesafe/jev-1.14", Decisions: spec,
			Input: prompts.Prompts{{Role: prompts.RoleUser, Content: "first"}, {Role: prompts.RoleUser, Content: "second"}},
		})
		require.NoError(t, err)
		require.Equal(t, "first\n\nsecond", got.body["state"])
	})
}

func TestDecisionsClient_InvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"NotJSON":       `not json`,
		"NoAnswers":     `{"answers": {}}`,
		"MissingType":   `{"answers": {"a": {"noul": 0.5}}}`,
		"AnswerNotJSON": `{"answers": {"a": 5}}`,
	} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer ts.Close()

			client := newClient(t, ts, newCatalog())
			_, err := client.Generate(context.Background(), &provider.Request{
				Model:     "typesafe/jev-1.14",
				Input:     userInput("text"),
				Decisions: &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}},
			})
			require.ErrorIs(t, err, errors.ErrInvalidDecisionsResponse)
		})
	}

	t.Run("UpstreamError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"bad question"}`, http.StatusBadRequest)
		}))
		defer ts.Close()

		client := newClient(t, ts, newCatalog())
		_, err := client.Generate(context.Background(), &provider.Request{
			Model:     "typesafe/jev-1.14",
			Input:     userInput("text"),
			Decisions: &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionNoul}}},
		})
		require.ErrorIs(t, err, errors.ErrRequestFailed)
	})
}

func TestNewDecisions(t *testing.T) {
	t.Run("RequiresCatalog", func(t *testing.T) {
		_, err := openrouter.NewDecisions(provider.Config{InferenceEndpoint: "https://openrouter.ai/api/v1"}, provider.ResolveOptions(), nil)
		require.ErrorIs(t, err, errors.ErrCatalogRequired)
	})

	t.Run("RegisteredForOpenRouter", func(t *testing.T) {
		registration, ok := provider.LookupRegistration(provider.ProviderTypeOpenRouter)
		require.True(t, ok)
		require.Contains(t, registration.APITypes, provider.APITypeDecisions)
	})

	t.Run("ProviderUsesDecisionsGenerator", func(t *testing.T) {
		client, err := openrouter.NewProvider(provider.Config{
			APIType:           provider.APITypeDecisions,
			ProviderType:      provider.ProviderTypeOpenRouter,
			InferenceEndpoint: "https://openrouter.ai/api/v1",
			CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
			Credentials:       auth.NewAPIKey("test-key"),
		}, provider.ResolveOptions())
		require.NoError(t, err)
		require.IsType(t, &openrouter.DecisionsClient{}, client.(*openrouter.Provider).Generator)
	})
}

func TestDecisionsAPIType(t *testing.T) {
	require.Equal(t, modality.Text, provider.APITypeDecisions.InputModality())
	require.Equal(t, modality.Decisions, provider.APITypeDecisions.OutputModality())
	require.Equal(t, "Decisions", provider.APITypeDecisions.DisplayName())
}

// The shape of the real models API response for Clairic, captured from
// GET https://openrouter.ai/api/v1/model/cloudflare/clairic.
const clairicModelJSON = `{
	"id": "cloudflare/clairic",
	"canonical_slug": "cloudflare/clairic-20261029",
	"name": "Cloudflare: Clairic",
	"created": 1793737067,
	"description": "Clairic is Cloudflare's System One-style extraction model.",
	"context_length": 65536,
	"architecture": {
		"modality": "text+image->decisions",
		"input_modalities": ["text", "image"],
		"output_modalities": ["decisions"],
		"tokenizer": "Other",
		"instruct_type": null
	},
	"pricing": {"prompt": "0.0000001", "completion": "0"},
	"top_provider": {"context_length": 65536, "max_completion_tokens": 58982, "is_moderated": false},
	"per_request_limits": null,
	"supported_parameters": [],
	"links": {"details": "/api/v1/models/cloudflare/clairic-20261029/endpoints"},
	"endpoints": ["/api/alpha/extractions"]
}`

func TestCatalog_DecisionModelEndpoints(t *testing.T) {
	client := testCatalogClient(t)

	model, err := client.DecodeModelJSON([]byte(clairicModelJSON))
	require.NoError(t, err)

	// A decision model by modality, served on the extractions route.
	require.Equal(t, modality.Decisions, model.OutputModality)
	require.Equal(t, modality.Text|modality.Image, model.InputModality)
	require.Equal(t, []string{"/api/alpha/extractions"}, model.Endpoints)

	// Models that do not publish endpoints decode with none.
	plain, err := client.DecodeModelJSON([]byte(exampleModelJSON))
	require.NoError(t, err)
	require.Empty(t, plain.Endpoints)
}
