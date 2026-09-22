package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/media"
	api "go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/auth/credtest"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// Exercises the Chat Completions endpoint against a live OpenRouter model when
// explicitly enabled.
func TestChatIntegration(t *testing.T) {
	// FIXME: fix test
	t.Skip("skipping tests due to rate limits and inconsistent behavior")

	// This test uses an open router SDK key to test the chat completions API.
	// This is an integration test, so use -short locally to skip the test.
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	conf := config.Provider{
		APIType:           types.APITypeOpenAIChatCompletions,
		ProviderType:      types.ProviderTypeOpenRouter,
		Credentials:       credtest.APIKey(testenv.OpenRouterAPIKey(t)),
		InferenceEndpoint: testenv.OpenRouterEndpointURL(t),
	}

	client, err := openai.NewChatCompletions(conf)
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "What is the capital of France?"},
			},
		}

		rep, model := generateWithModels(t, client, testenv.OpenRouterTextModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		// Check the response
		require.NotEmpty(t, rep.ID)
		require.Empty(t, rep.Attachments)
		require.NotZero(t, rep.Created)
		require.True(t, strings.HasPrefix(rep.Model, model), "expected model to start with %q but got %s", model, rep.Model)
		require.NotZero(t, rep.Usage)

		// Check the response output
		require.Len(t, rep.Output, 1)
		output := rep.Output[0]

		require.Empty(t, output.ID)
		require.Equal(t, api.RoleAssistant, output.Role)
		require.Equal(t, api.OutputMessage, output.Type)
		require.NotEmpty(t, output.Content)
		require.Empty(t, output.Citations)
	})

	t.Run("OutputSchema", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleSystem, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "Respond with a JSON object containing at least 2 colors and no more than 8 colors using the defined json schema."},
			},
			OutputSchema: &schema.Schema{
				Name:     "colors",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Colors",
					"type": "object",
					"properties": {
						"colors": {
							"type": "array",
							"items": {
								"type": "object",
								"description": "A color with a name and rgb value in hex format",
								"properties": {
									"name": {
										"type": "string",
										"description": "The name of the color"
									},
									"rgb": {
										"type": "string",
										"description": "The value of the color in hex format (e.g. #000000)"
									}
								},
								"required": ["name", "rgb"]
							}
						}
					},
					"required": ["colors"]
				}`,
			},
		}

		rep, _ := generateWithModels(t, client, testenv.OpenRouterTextModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)

		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var colors map[string]any
		err = json.Unmarshal([]byte(output.Content), &colors)
		require.NoError(t, err)
	})

	t.Run("TextAttachment", func(t *testing.T) {
		// FIXME: fix test
		t.Skip("skipping test due to slow responses")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "What is the topic of the attached text file? Respond with a JSON object containing the topic."},
			},
			OutputSchema: &schema.Schema{
				Name:     "topic",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Topic",
					"type": "object",
					"properties": {
						"topic": {
							"type": "string",
							"description": "The topic of the attached text file"
						}
					},
					"required": ["topic"]
				}`,
			},
			Attachments: []*api.Attachment{
				{
					MimeType: media.TextHTML,
					Filename: "sample.html",
					URL:      "https://rotational.io",
				},
			},
		}

		rep, _ := generateWithModels(t, client, testenv.OpenRouterMultimodalModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)
		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var content map[string]any
		err = json.Unmarshal([]byte(output.Content), &content)
		require.NoError(t, err)
		require.NotEmpty(t, content["topic"])
	})

	t.Run("ImageAttachment", func(t *testing.T) {
		// FIXME: fix test
		t.Skip("skipping test due to slow responses")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "Which state flag is pictured in the image? Respond with a JSON object containing the name of the state."},
			},
			OutputSchema: &schema.Schema{
				Name:     "flag",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Flag",
					"type": "object",
					"properties": {
						"name": {
							"type": "string",
							"description": "The name of the state"
						}
					},
					"required": ["name"]
				}`,
			},
			Attachments: []*api.Attachment{
				{
					MimeType: media.ImagePNG,
					Filename: "flag.png",
					URL:      "https://ballotpedia.org/File:Flag_of_Minnesota.png",
				},
			},
		}

		rep, _ := generateWithModels(t, client, testenv.OpenRouterMultimodalModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)
		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var flag map[string]any
		err = json.Unmarshal([]byte(output.Content), &flag)
		require.NoError(t, err)
		require.NotEmpty(t, flag["name"])
	})

	t.Run("AudioAttachment", func(t *testing.T) {
		// FIXME: fix test
		t.Skip("skipping test due to slow responses")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "Transcribe the first 10 seconds of the audio file. Respond with a JSON object containing the transcription."},
			},
			OutputSchema: &schema.Schema{
				Name:     "transcription",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Transcription",
					"type": "object",
					"properties": {
						"transcription": {
							"type": "string",
							"description": "The transcription of the audio file"
						}
					},
					"required": ["transcription"]
				}`,
			},
			Attachments: []*api.Attachment{
				{
					MimeType: media.AudioMPEG,
					Filename: "sample.mp3",
					URL:      "https://samplelib.com/mp3/sample-speech-1m.mp3",
				},
			},
		}

		rep, _ := generateWithModels(t, client, testenv.OpenRouterMultimodalModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)
		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var transcription map[string]any
		err = json.Unmarshal([]byte(output.Content), &transcription)
		require.NoError(t, err)
		require.NotEmpty(t, transcription["transcription"])
	})

	t.Run("FileAttachment", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &api.Request{
			Model: "", // set in generateWithModels
			Input: []api.Message{
				{Role: api.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: api.RoleUser, Content: "What topic is described by the attached file? Respond with a JSON object containing the topic."},
			},
			OutputSchema: &schema.Schema{
				Name:     "topic",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Topic",
					"type": "object",
					"properties": {
						"topic": {
							"type": "string",
							"description": "The topic of the attached file"
						}
					},
					"required": ["topic"]
				}`,
			},
			Attachments: []*api.Attachment{
				{
					MimeType: media.ApplicationPDF,
					Filename: "sample.pdf",
					URL:      "https://bitcoin.org/bitcoin.pdf",
				},
			},
		}

		rep, _ := generateWithModels(t, client, testenv.OpenRouterTextModels, ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)
		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var topic map[string]any
		err = json.Unmarshal([]byte(output.Content), &topic)
		require.NoError(t, err)
		require.NotEmpty(t, topic["topic"])
	})
}

// Verifies tool definitions and tool transcripts map to the Chat Completions
// request format.
func TestChatToolMapping(t *testing.T) {
	// Set up a request containing a tool definition, an assistant call, and its
	// corresponding result.
	integrationID := ulid.Make()
	// Build the provider request and assert all tool metadata survives mapping.
	body, err := openai.ChatCompletionsBody(&api.Request{
		Model: "test-model",
		Input: []api.Message{
			{Role: api.RoleUser, Content: "lookup"},
			{
				Role: api.RoleAssistant,
				ToolCalls: []capabilities.ToolCall{{
					CallID:    "call-1",
					Name:      "lookup",
					Arguments: json.RawMessage(`{"key":"value"}`),
				}},
			},
			{
				Role: api.RoleTool,
				ToolResults: []capabilities.ToolResult{{
					CallID:  "call-1",
					Content: []capabilities.Content{{Type: "text", Text: "42"}},
				}},
			},
		},
		Tools: []capabilities.ToolDefinition{{
			IntegrationID: integrationID,
			Name:          "lookup",
			Description:   "Look up a value.",
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}}}`),
		}},
	})

	require.NoError(t, err)
	require.Len(t, body.Tools, 1)
	require.Equal(t, "auto", body.ToolChoice.OfAuto.Value)
	require.True(t, body.ParallelToolCalls.Value)
	require.Equal(t, "lookup", body.Tools[0].OfFunction.Function.Name)
	require.Equal(t, "Look up a value.", body.Tools[0].OfFunction.Function.Description.Value)
	require.Equal(t, "object", body.Tools[0].OfFunction.Function.Parameters["type"])
	require.Len(t, body.Messages, 3)
	require.Len(t, body.Messages[1].OfAssistant.ToolCalls, 1)
	require.Equal(t, "call-1", body.Messages[1].OfAssistant.ToolCalls[0].OfFunction.ID)
	require.Equal(t, "call-1", body.Messages[2].OfTool.ToolCallID)
	require.Equal(t, "42", body.Messages[2].OfTool.Content.OfString.Value)
}

// A tool message must include a result and its call ID.
func TestChatToolMessageRequiresResult(t *testing.T) {
	_, err := openai.ChatCompletionsMessages([]api.Message{{
		Role:    api.RoleTool,
		Content: "42",
	}}, nil)

	require.EqualError(t, err, "tool message must contain tool results")
}

// Verifies a Chat Completions function call is converted into a provider-neutral
// Horizon tool call.
func TestChatGenerateMapsToolCalls(t *testing.T) {
	// Return a minimal API response from a local server so the test covers the
	// actual SDK response decoding path without network access.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":1735689600,
			"model":"test-model",
			"choices":[{
				"index":0,
				"finish_reason":"tool_calls",
				"message":{
					"role":"assistant",
					"content":null,
					"tool_calls":[{
						"id":"call-1",
						"type":"function",
						"function":{"name":"lookup","arguments":"{\"key\":\"value\"}"}
					}]
				}
			}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`))
	}))
	defer server.Close()

	client, err := openai.NewChatCompletions(config.Provider{
		InferenceEndpoint: server.URL + "/",
		Credentials:       credtest.APIKey("test-key"),
	})
	require.NoError(t, err)

	// Execute one request and assert the normalized call ID, name, arguments,
	// and token usage.
	response, err := client.Generate(context.Background(), &api.Request{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.ToolCalls, 1)
	require.Equal(t, "call-1", response.ToolCalls[0].CallID)
	require.Equal(t, "lookup", response.ToolCalls[0].Name)
	require.JSONEq(t, `{"key":"value"}`, string(response.ToolCalls[0].Arguments))
	require.Equal(t, int64(5), response.Usage.TotalTokens)
}

// Tries each model in order and advances to the next one when OpenRouter
// rate-limits the request.
func generateWithModels(t *testing.T, client interface {
	Generate(context.Context, *api.Request) (*api.Response, error)
}, models []string, ctx context.Context, req *api.Request) (*api.Response, string) {
	t.Helper()

	var response *api.Response
	model := testenv.RunWithModels(t, models, func(model string) error {
		req.Model = model
		var err error
		response, err = client.Generate(ctx, req)
		return err
	})
	return response, model
}
