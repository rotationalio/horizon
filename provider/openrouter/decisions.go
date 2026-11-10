package openrouter

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
)

// OpenRouter's alpha routes for typed answers, as listed in a model's "endpoints".
const (
	// Decision models take a "questions" map.
	routeDecisions = "/api/alpha/decisions"

	// Extraction models take a "fields" map. EDGE CASE: cloudflare/clairic has the
	// "decisions" modality but is only served here; /api/alpha/decisions rejects it.
	routeExtractions = "/api/alpha/extractions"
)

// How long a model's route is remembered before the catalog is asked again.
const routeCacheTTL = 5 * time.Minute

// modelRetriever looks up one model in the provider catalog.
type modelRetriever interface {
	RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error)
}

// DecisionsClient implements [provider.Generator] for OpenRouter's typed-answer
// routes, choosing the decisions or extractions route per model from the catalog.
type DecisionsClient struct {
	endpoint *http.Endpoint
	catalog  modelRetriever

	mu     sync.Mutex
	routes map[string]cachedRoute
	now    func() time.Time
}

type cachedRoute struct {
	route   string
	expires time.Time
}

var _ provider.Generator = (*DecisionsClient)(nil)

// NewDecisions creates a decisions client from the provider configuration. The
// catalog is used to look up which route each model is served on.
func NewDecisions(conf provider.Config, options provider.Options, models modelRetriever) (*DecisionsClient, error) {
	if models == nil {
		return nil, errors.ErrCatalogRequired
	}

	base, err := decisionsBaseURL(conf.InferenceEndpoint)
	if err != nil {
		return nil, err
	}

	endpoint, err := http.NewEndpoint(base, options.HTTPClient, conf.Credentials)
	if err != nil {
		return nil, err
	}

	return &DecisionsClient{
		endpoint: endpoint,
		catalog:  models,
		routes:   make(map[string]cachedRoute),
		now:      time.Now,
	}, nil
}

// The alpha routes sit beside /api/v1, not under it, so swap a trailing /v1 for /alpha.
func decisionsBaseURL(inference string) (string, error) {
	parsed, err := url.Parse(inference)
	if err != nil {
		return "", errors.Fmt("%w: %w", errors.ErrInvalidInferenceEndpoint, err)
	}

	path := strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), "/v1")
	parsed.Path = path + "/alpha"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	return parsed.String(), nil
}

// Generate sends the request's Questions (decisions models) or Fields (extractions
// models) and returns typed answers. Input must be user prompts only; they are
// joined into the "state", since the questions or fields replace a system prompt.
func (c *DecisionsClient) Generate(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if req == nil {
		return nil, errors.New("provider request is required")
	}
	if err := req.Decisions.Validate(); err != nil {
		return nil, errors.Join(errors.ErrDecisionSpecRequired, err)
	}

	state, err := decisionState(req)
	if err != nil {
		return nil, err
	}

	route, err := c.route(ctx, req.Model)
	if err != nil {
		return nil, err
	}

	var body any
	switch route {
	case routeDecisions:
		if len(req.Decisions.Questions) == 0 {
			return nil, errors.Fmt("%w: model %s is a decisions model and needs questions", errors.ErrDecisionSpecRequired, req.Model)
		}
		body = decisionsBody{Model: req.Model, State: state, Questions: req.Decisions.Questions}
	case routeExtractions:
		if len(req.Decisions.Fields) == 0 {
			return nil, errors.Fmt("%w: model %s is an extractions model and needs fields", errors.ErrDecisionSpecRequired, req.Model)
		}
		body = extractionsBody{Model: req.Model, State: state, Fields: req.Decisions.Fields, ReferenceDate: req.Decisions.ReferenceDate}
	}

	// The endpoint base ends in /alpha, so the route's last segment is the path.
	raw, err := c.endpoint.PostJSON(ctx, strings.TrimPrefix(route, "/api/alpha/"), body)
	if err != nil {
		return nil, err
	}

	resp, err := decodeDecisions(raw, route, c.now())
	if err != nil {
		return nil, err
	}

	// Callers look the model up by the slug they requested, so keep it and note what served it.
	if resp.Model != "" && resp.Model != req.Model {
		resp.Meta.PutString("served_model", resp.Model)
	}
	resp.Model = req.Model
	return resp, nil
}

// Picks the route for a model from the catalog, caching the result briefly so each
// Generate call does not cost an extra catalog request.
func (c *DecisionsClient) route(ctx context.Context, modelID string) (string, error) {
	now := c.now()

	c.mu.Lock()
	cached, ok := c.routes[modelID]
	c.mu.Unlock()
	if ok && now.Before(cached.expires) {
		return cached.route, nil
	}

	model, err := c.catalog.RetrieveModel(ctx, modelID)
	if err != nil {
		return "", err
	}

	route, err := selectRoute(model)
	if err != nil {
		return "", errors.Fmt("model %s: %w", modelID, err)
	}

	c.mu.Lock()
	c.routes[modelID] = cachedRoute{route: route, expires: now.Add(routeCacheTTL)}
	c.mu.Unlock()
	return route, nil
}

// Chooses the route from index 0 of the model's endpoints. Every decision model
// lists exactly one today; revisit this if one ever lists several.
func selectRoute(model *catalog.Model) (string, error) {
	if model == nil || len(model.Endpoints) == 0 {
		return "", errors.ErrNoModelEndpoints
	}

	switch route := model.Endpoints[0]; route {
	case routeDecisions, routeExtractions:
		return route, nil
	default:
		return "", errors.Fmt("%w: %s", errors.ErrUnsupportedModelEndpoint, route)
	}
}

// Builds the "state" string from the request input. Text and user prompts only.
func decisionState(req *provider.Request) (string, error) {
	if len(req.Attachments) > 0 {
		return "", errors.Fmt("%w: attachments", errors.ErrUnsupportedDecisionInput)
	}
	if len(req.Tools) > 0 {
		return "", errors.Fmt("%w: tools", errors.ErrUnsupportedDecisionInput)
	}
	if req.OutputSchema != nil {
		return "", errors.Fmt("%w: output schema", errors.ErrUnsupportedDecisionInput)
	}

	parts := make([]string, 0, len(req.Input))
	for _, prompt := range req.Input {
		if prompt == nil {
			continue
		}
		if prompt.Role != prompts.RoleUser {
			return "", errors.Fmt("%w: %s prompts (use the request questions or fields instead)", errors.ErrUnsupportedDecisionInput, prompt.Role)
		}
		parts = append(parts, prompt.Content)
	}

	state := strings.Join(parts, "\n\n")
	if strings.TrimSpace(state) == "" {
		return "", errors.ErrMissingInput
	}
	return state, nil
}

//=============================================================================
// Wire types
//=============================================================================

// decisionsBody is the request body for /api/alpha/decisions.
type decisionsBody struct {
	Model     string                       `json:"model"`
	State     string                       `json:"state"`
	Questions map[string]provider.Question `json:"questions"`
}

// extractionsBody is the request body for /api/alpha/extractions.
type extractionsBody struct {
	Model         string                    `json:"model"`
	State         string                    `json:"state"`
	Fields        map[string]provider.Field `json:"fields"`
	ReferenceDate string                    `json:"reference_date,omitempty"`
}

// decisionsWire is the response shape shared by both routes. Each answer's shape
// depends on its type, so answers are kept raw.
type decisionsWire struct {
	ID        string                     `json:"id"`
	Model     string                     `json:"model"`
	Provider  string                     `json:"provider"`
	Answers   map[string]json.RawMessage `json:"answers"`
	Truncated bool                       `json:"truncated"`
	Usage     struct {
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
}

func decodeDecisions(data []byte, route string, created time.Time) (*provider.Response, error) {
	var wire decisionsWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, errors.Join(errors.ErrInvalidDecisionsResponse, err)
	}
	if len(wire.Answers) == 0 {
		return nil, errors.Join(errors.ErrInvalidDecisionsResponse, errors.ErrNoModelOutput)
	}

	answers := make(provider.Answers, len(wire.Answers))
	for name, raw := range wire.Answers {
		// Every answer carries a type; a missing one is an error, not a default.
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &kind); err != nil || kind.Type == "" {
			return nil, errors.Fmt("%w: answer %q has no type", errors.ErrInvalidDecisionsResponse, name)
		}
		answers[name] = provider.Answer{Type: kind.Type, Raw: raw}
	}

	// Also put the answers in the output as JSON for consumers that only read prompts.
	content, err := json.Marshal(wire.Answers)
	if err != nil {
		return nil, err
	}

	out := &provider.Response{
		ID:      wire.ID,
		Model:   wire.Model,
		Created: created,
		Usage: provider.Usage{
			InputTokens:  wire.Usage.InputTokens,
			OutputTokens: wire.Usage.OutputTokens,
			TotalTokens:  wire.Usage.InputTokens + wire.Usage.OutputTokens,
			APICost:      wire.Usage.Cost,
		},
		Output: prompts.Prompts{{
			Role:    prompts.RoleAssistant,
			Type:    prompts.TypeMessage,
			Content: string(content),
		}},
		Answers: answers,
		Meta:    make(provider.Meta),
	}
	out.Meta.PutString("provider", wire.Provider)
	out.Meta.PutString("route", route)
	if wire.Truncated {
		out.Meta["truncated"] = true
	}
	return out, nil
}
