package openai_test

import (
	"testing"
)

// Tool calling is intentionally deferred until the provider-neutral tool model
// is implemented.
func TestToolCallingIntegration(t *testing.T) {
	t.Skip("tool calling is not implemented yet")
}

// TODO: Re-enable the live tool-calling suites once the provider-neutral tool
// model and request loop are implemented.
/*
// Exercises a complete two-request Chat Completions tool exchange through
// OpenRouter, including request serialization and response deserialization.
func TestChatCompletionsToolCallingIntegration(t *testing.T) {
	t.Skip("skipping tests due to rate limits and inconsistent behavior")

	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	conf := openRouterToolTestConfig(t, types.APITypeOpenAIChatCompletions)
	client, err := openai.NewChatCompletions(conf)
	require.NoError(t, err)

	tool := openRouterTestTool()
	request := openRouterToolRequest(tool)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	first, err := client.Generate(ctx, request)
	require.NoError(t, err)
	call := requireToolCall(t, first, tool)

	appendFakeToolExchange(request, first, call, "Paris is the capital of France.")
	final, err := client.Generate(ctx, request)
	require.NoError(t, err)
	requireFinalToolAnswer(t, final, "Paris")
}

// Exercises the same complete exchange through the OpenAI Responses adapter
// routed to OpenRouter.
func TestResponsesToolCallingIntegration(t *testing.T) {
	t.Skip("skipping tests due to rate limits and inconsistent behavior")

	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	conf := openRouterToolTestConfig(t, types.APITypeOpenAIResponses)
	client, err := openai.NewResponses(conf)
	require.NoError(t, err)

	tool := openRouterTestTool()
	request := openRouterToolRequest(tool)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	first, err := client.Generate(ctx, request)
	require.NoError(t, err)
	call := requireToolCall(t, first, tool)

	appendFakeToolExchange(request, first, call, "Paris is the capital of France.")
	final, err := client.Generate(ctx, request)
	require.NoError(t, err)
	requireFinalToolAnswer(t, final, "Paris")
}

// Loads the same OpenRouter credentials and endpoint used by the other live
// integration tests in this package.
func openRouterToolTestConfig(t *testing.T, apiType types.APIType) config.Provider {
	t.Helper()
	conftest.LoadEnv(t)
	conftest.InstallConfig(t, "")
	return config.Provider{
		APIType:           apiType,
		ProviderType:      types.ProviderTypeOpenRouter,
		Credentials:       credtest.APIKey(conftest.RequireOpenRouterAPIKey(t)),
		InferenceEndpoint: conftest.RequireOpenRouterEndpointURL(t),
	}
}

// Creates one fake function tool whose result is supplied by the test rather
// than by a real capability provider.
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

func openRouterToolRequest(tool capabilities.ToolDefinition) *horizon.Request {
	return &horizon.Request{
		Model: openrouterFreeModel,
		Tools: []capabilities.ToolDefinition{tool},
		Input: []horizon.Message{{
			Role:    horizon.RoleUser,
			Content: "What is the capital of France? You must call get_capital before answering and should not answer from memory.",
		}},
	}
}

func requireToolCall(t *testing.T, response *horizon.Response, tool capabilities.ToolDefinition) capabilities.ToolCall {
	t.Helper()
	require.NotNil(t, response)
	require.Len(t, response.ToolCalls, 1)

	call := response.ToolCalls[0]
	require.Equal(t, tool.Name, call.Name)
	require.NotEmpty(t, call.CallID)
	require.True(t, json.Valid(call.Arguments), "tool arguments must be valid JSON: %q", call.Arguments)
	return call
}

func appendFakeToolExchange(request *horizon.Request, response *horizon.Response, call capabilities.ToolCall, result string) {
	var assistantContent string
	for _, output := range response.Output {
		if output.Role == horizon.RoleAssistant {
			assistantContent += output.Content
		}
	}

	request.Input = append(request.Input,
		horizon.Message{
			Role:      horizon.RoleAssistant,
			Content:   assistantContent,
			ToolCalls: response.ToolCalls,
		},
		horizon.Message{
			Role:      horizon.RoleTool,
			ToolCalls: []capabilities.ToolCall{call},
			ToolResults: []capabilities.ToolResult{{
				CallID:  call.CallID,
				Content: []capabilities.Content{{Type: "text", Text: result}},
			}},
		},
	)
}

func requireFinalToolAnswer(t *testing.T, response *horizon.Response, expected string) {
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
*/
