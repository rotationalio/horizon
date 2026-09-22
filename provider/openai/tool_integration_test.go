package openai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/prompts"
	api "go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/auth/credtest"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
)

// Exercises a complete two-request Chat Completions tool exchange through
// OpenRouter, including request serialization and response deserialization.
func TestChatCompletionsToolCallingIntegration(t *testing.T) {
	// FIXME: fix test
	testenv.RunUnstable(t)

	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	runToolCallingIntegration(t, types.APITypeOpenAIChatCompletions, testenv.OpenRouterTextModels)
}

// Exercises the same complete exchange through the OpenAI Responses adapter
// routed to OpenRouter.
func TestResponsesToolCallingIntegration(t *testing.T) {
	// FIXME: fix test
	testenv.RunUnstable(t)

	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	runToolCallingIntegration(t, types.APITypeOpenAIResponses, testenv.OpenRouterTextModels)
}

// Runs a complete tool-calling exchange through the specified API type and
// OpenRouter models, retrying rate limit errors.
func runToolCallingIntegration(t *testing.T, apiType types.APIType, models []string) {
	t.Helper()
	testenv.RunWithModels(t, models, func(model string) error {
		conf := openRouterToolTestConfig(t, apiType)
		var client interface {
			Generate(context.Context, *api.Request) (*api.Response, error)
		}
		var err error
		if apiType == types.APITypeOpenAIChatCompletions {
			client, err = openai.NewChatCompletions(conf)
		} else {
			client, err = openai.NewResponses(conf)
		}
		if err != nil {
			return testenv.ModelError(model, err)
		}

		tool := openRouterTestTool()
		request := openRouterToolRequest(tool, model)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		first, err := client.Generate(ctx, request)
		if err != nil {
			return testenv.ModelError(model, err)
		}
		call := requireToolCall(t, first, tool)

		appendFakeToolExchange(request, first, call, "Paris is the capital of France.")
		final, err := client.Generate(ctx, request)
		if err != nil {
			return testenv.ModelError(model, err)
		}
		requireFinalToolAnswer(t, final, "Paris")
		return nil
	})
}

// Loads the same OpenRouter credentials and endpoint used by the other live
// integration tests in this package.
func openRouterToolTestConfig(t *testing.T, apiType types.APIType) config.Provider {
	t.Helper()
	return config.Provider{
		APIType:           apiType,
		ProviderType:      types.ProviderTypeOpenRouter,
		Credentials:       credtest.APIKey(testenv.OpenRouterAPIKey(t)),
		InferenceEndpoint: testenv.OpenRouterEndpointURL(t),
	}
}

// Creates one fake function tool whose result is supplied by the test rather
// than by a real capability api.
func openRouterTestTool() capabilities.ToolDefinition {
	return capabilities.ToolDefinition{
		ToolID:        ulid.Make(),
		IntegrationID: ulid.Make(),
		Name:          "get_capital",
		Description:   "Return the capital city for a country. You must use this tool to answer the user's question.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"country": {"type": "string", "description": "The country to look up"}
			},
			"required": ["country"],
			"additionalProperties": false
		}`),
	}
}

func openRouterToolRequest(tool capabilities.ToolDefinition, model string) *api.Request {
	return &api.Request{
		Model: model,
		Tools: []capabilities.ToolDefinition{tool},
		Input: []api.Message{{
			Role:    api.RoleUser,
			Content: "What is the capital of France? You must call get_capital before answering and should not answer from memory.",
		}},
	}
}

func requireToolCall(t *testing.T, response *api.Response, tool capabilities.ToolDefinition) capabilities.ToolCall {
	t.Helper()
	require.NotNil(t, response)
	require.Len(t, response.ToolCalls, 1)

	call := response.ToolCalls[0]
	require.Equal(t, tool.Name, call.Name)
	require.NotEmpty(t, call.CallID)
	require.True(t, json.Valid(call.Arguments), "tool arguments must be valid JSON: %q", call.Arguments)
	return call
}

func appendFakeToolExchange(request *api.Request, response *api.Response, call capabilities.ToolCall, result string) {
	var assistantContent string
	for _, output := range response.Output {
		if output.Role == api.RoleAssistant {
			assistantContent += output.Content
		}
	}

	request.Input = append(request.Input,
		&prompts.Prompt{
			Role:      api.RoleAssistant,
			Content:   assistantContent,
			ToolCalls: response.ToolCalls,
		},
		&prompts.Prompt{
			Role:      api.RoleTool,
			ToolCalls: []capabilities.ToolCall{call},
			ToolResults: []capabilities.ToolResult{{
				CallID:  call.CallID,
				Content: []capabilities.Content{{Type: "text", Text: result}},
			}},
		},
	)
}

func requireFinalToolAnswer(t *testing.T, response *api.Response, expected string) {
	t.Helper()
	require.NotNil(t, response)
	require.Empty(t, response.ToolCalls)
	require.NotEmpty(t, response.Output)

	var content strings.Builder
	for _, output := range response.Output {
		content.WriteString(output.Content)
	}
	require.Contains(t, strings.ToLower(content.String()), strings.ToLower(expected))
}
