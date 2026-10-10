package openai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
)

// Exercises a complete two-request Chat Completions tool exchange through
// OpenRouter, including request serialization and response deserialization.
func TestChatCompletionsToolCallingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	// Setup: use the real adapter and one locally implemented function tool.
	client, err := openai.NewChatCompletions(openRouterToolTestConfig(t), provider.ResolveOptions())
	require.NoError(t, err)
	tool := openRouterTestTool()
	request := openRouterToolRequest(t, tool)

	// Execute the first request and assert the model returned a usable call.
	first := generateToolRequest(t, client, request)
	call := requireToolCall(t, first, tool)

	// Supply the result locally; this tests adapter replay, not Horizon orchestration.
	appendFakeToolExchange(request, first, call, "Paris is the capital of France.")
	final := generateToolRequest(t, client, request)

	// Assert the second response answers the question without requesting more tools.
	requireFinalToolAnswer(t, final, "Paris")
}

// Exercises the same complete exchange through the OpenAI Responses adapter
// routed to OpenRouter.
func TestResponsesToolCallingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OpenRouter tool-calling test in short mode")
	}

	// Setup: use the real adapter and one locally implemented function tool.
	client, err := openai.NewResponses(openRouterToolTestConfig(t), provider.ResolveOptions())
	require.NoError(t, err)
	tool := openRouterTestTool()
	request := openRouterToolRequest(t, tool)

	// Execute the first request and assert the model returned a usable call.
	first := generateToolRequest(t, client, request)
	call := requireToolCall(t, first, tool)

	// Preserve all provider items, including reasoning needed for continuation.
	appendFakeToolExchange(request, first, call, "Paris is the capital of France.")
	final := generateToolRequest(t, client, request)

	// Assert the second response answers the question without requesting more tools.
	requireFinalToolAnswer(t, final, "Paris")
}

// Returns the test provider config for OpenRouter.
func openRouterToolTestConfig(t *testing.T) provider.Config {
	t.Helper()
	return provider.Config{
		Credentials:       auth.NewAPIKey(testenv.OpenRouterAPIKey(t)),
		InferenceEndpoint: testenv.OpenRouterEndpointURL(t),
	}
}

// Returns one strict function tool whose result is supplied by the test rather
// than by a real capability provider.
func openRouterTestTool() capabilities.Definition {
	return capabilities.Definition{
		Name:        "get_capital",
		Description: "Return the capital city for a country. You must use this tool to answer the user's question.",
		InputSchema: &schema.Schema{
			Name:     "get_capital_arguments",
			Strict:   true,
			MimeType: mime.ApplicationSchemaJSON,
			Data: `{
				"type": "object",
				"properties": {
					"country": {"type": "string", "description": "The country to look up"}
				},
				"required": ["country"],
				"additionalProperties": false
			}`,
		},
	}
}

// Builds a request that requires the configured model to call the test tool.
func openRouterToolRequest(t *testing.T, tool capabilities.Definition) *provider.Request {
	t.Helper()
	models := testenv.OpenRouterTextModels(t)
	return &provider.Request{
		Model: models[0],
		Tools: []capabilities.Definition{tool},
		Input: prompts.Prompts{{
			Role:    prompts.RoleUser,
			Content: "What is the capital of France? You must call get_capital before answering and should not answer from memory.",
		}},
	}
}

// Generates one tool-test response with a bounded timeout and fails the test on errors.
func generateToolRequest(t *testing.T, client provider.Generator, request *provider.Request) *provider.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
	defer cancel()

	response, err := client.Generate(ctx, request)
	require.NoError(t, err)
	return response
}

// Extracts and validates the single function call returned by the model.
func requireToolCall(t *testing.T, response *provider.Response, tool capabilities.Definition) prompts.ToolCall {
	t.Helper()
	require.NotNil(t, response)

	var calls []prompts.ToolCall
	for _, output := range response.Output {
		calls = append(calls, output.ToolCalls...)
	}
	require.Len(t, calls, 1)

	call := calls[0]
	require.Equal(t, string(tool.Name), call.Name)
	require.NotEmpty(t, call.CallID)
	require.True(t, json.Valid([]byte(call.Arguments)), "tool arguments must be valid JSON: %q", call.Arguments)
	return call
}

// Appends the provider transcript and a locally fabricated tool result for the next turn.
func appendFakeToolExchange(request *provider.Request, response *provider.Response, call prompts.ToolCall, result string) {
	request.Input = append(request.Input, response.Output...)
	request.Input = append(request.Input, &prompts.Prompt{
		Role:        prompts.RoleTool,
		Type:        prompts.TypeMessage,
		ToolResults: []prompts.ToolResponse{{CallID: call.CallID, Content: result}},
	})
}

// Verifies the final response contains the expected answer and no additional tool calls.
func requireFinalToolAnswer(t *testing.T, response *provider.Response, expected string) {
	t.Helper()
	require.NotNil(t, response)
	require.NotEmpty(t, response.Output)

	var content strings.Builder
	for _, output := range response.Output {
		require.Empty(t, output.ToolCalls)
		if output.Type != prompts.TypeReasoning {
			content.WriteString(output.Content)
		}
	}
	require.Contains(t, strings.ToLower(content.String()), strings.ToLower(expected))
}
