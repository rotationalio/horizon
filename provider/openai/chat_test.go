package openai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// Exercises the Chat Completions endpoint against live OpenRouter models.
func TestChatIntegration(t *testing.T) {
	// This test uses an open router SDK key to test the chat completions API.
	// This is an integration test, so use -short locally to skip the test.
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	conf := provider.Config{
		Credentials:       auth.NewAPIKey(testenv.OpenRouterAPIKey(t)),
		InferenceEndpoint: testenv.OpenRouterEndpointURL(t),
	}

	client, err := openai.NewChatCompletions(conf)
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "What is the capital of France?"},
			},
		}

		rep, model := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels)
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
		require.Equal(t, prompts.RoleAssistant, output.Role)
		require.Equal(t, prompts.TypeMessage, output.Type)
		require.NotEmpty(t, output.Content)
		require.Empty(t, output.Citations)
	})

	t.Run("OutputSchema", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleSystem, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "Respond with a JSON object containing at least 2 colors and no more than 8 colors using the defined json schema."},
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "What is the topic of the attached text file? Respond with a JSON object containing the topic."},
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
			Attachments: attachments.Attachments{
				{
					ContentType: string(mime.TextHTML),
					Filename:    "sample.html",
					URL:         "https://rotational.io",
				},
			},
		}

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "What animal is pictured in the attached image? Respond with a JSON object containing the animal."},
			},
			OutputSchema: &schema.Schema{
				Name:     "image_subject",
				MimeType: mime.ApplicationSchemaJSON,
				Version: semver.Version{
					Major: 1,
					Minor: 0,
					Patch: 0,
				},
				Data: `
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"title": "Image Subject",
					"type": "object",
					"properties": {
						"animal": {
							"type": "string",
							"description": "The animal pictured in the image"
						}
					},
					"required": ["animal"]
				}`,
			},
			Attachments: attachments.Attachments{
				{
					ContentType: string(mime.ImagePNG),
					Filename:    "pig.png",
					// httpbin provides deterministic HTTP test fixtures:
					// https://github.com/postmanlabs/httpbin
					URL: "https://httpbin.org/image/png",
				},
			},
		}

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels)
		require.NotNil(t, rep)

		require.Len(t, rep.Output, 1)
		output := rep.Output[0]
		require.NotEmpty(t, output.Content)

		// Attempt to parse the output as a JSON object
		var subject map[string]any
		err = json.Unmarshal([]byte(output.Content), &subject)
		require.NoError(t, err)
		animal, ok := subject["animal"].(string)
		require.True(t, ok)
		require.Contains(t, strings.ToLower(animal), "pig")
	})

	t.Run("AudioAttachment", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "Transcribe the first 10 seconds of the audio file. Respond with a JSON object containing the transcription."},
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
			Attachments: attachments.Attachments{
				{
					ContentType: string(mime.AudioMPEG),
					Filename:    "sample.mp3",
					URL:         "https://samplelib.com/mp3/sample-speech-1m.mp3",
				},
			},
		}

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels)
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

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "What topic is described by the attached file? Respond with a JSON object containing the topic."},
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
			Attachments: attachments.Attachments{
				{
					ContentType: string(mime.ApplicationPDF),
					Filename:    "sample.pdf",
					URL:         "https://bitcoin.org/bitcoin.pdf",
				},
			},
		}

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels)
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

// Verifies MP3 and WAV attachments use raw base64 with the format names expected
// by Chat Completions, and rejects unsupported audio formats.
func TestChatCompletionsAudioAttachments(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantFormat  string
	}{
		{name: "MP3", contentType: string(mime.AudioMPEG), wantFormat: "mp3"},
		{name: "WAV", contentType: "audio/wav", wantFormat: "wav"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts, err := openai.ChatCompletionsAttachments(attachments.Attachments{{
				ContentType: tc.contentType,
				Filename:    "sample." + tc.wantFormat,
				Data:        []byte("raw audio"),
			}})
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.NotNil(t, parts[0].OfInputAudio)
			require.Equal(t, "cmF3IGF1ZGlv", parts[0].OfInputAudio.InputAudio.Data)
			require.Equal(t, tc.wantFormat, parts[0].OfInputAudio.InputAudio.Format)
		})
	}

	t.Run("Unsupported", func(t *testing.T) {
		_, err := openai.ChatCompletionsAttachments(attachments.Attachments{{
			ContentType: "audio/ogg",
			Filename:    "sample.ogg",
			Data:        []byte("ogg"),
		}})
		require.ErrorContains(t, err, "unsupported Chat Completions audio content type")
	})
}

// Verifies cancellation is propagated while resolving Chat Completions
// attachments.
func TestChatCompletionsAttachmentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := openai.ChatCompletionsAttachmentsContext(ctx, attachments.Attachments{{
		ContentType: string(mime.ImagePNG),
		Filename:    "image.png",
		URL:         "https://example.com/image.png",
	}})
	require.ErrorIs(t, err, context.Canceled)
}

// TODO: Re-enable the tool mapping tests
/*
// Verifies tool definitions and tool transcripts map to the Chat Completions
// request format.
func TestChatToolMapping(t *testing.T) {
	// Set up a request containing a tool definition, an assistant call, and its
	// corresponding result.
	integrationID := ulid.Make()
	// Build the provider request and assert all tool metadata survives mapping.
	body, err := openai.ChatCompletionsBody(&horizon.Request{
		Model: "test-model",
		Input: prompts.Prompts{
			{Role: prompts.RoleUser, Content: "lookup"},
			{
				Role: prompts.RoleAssistant,
				ToolCalls: []capabilities.ToolCall{{
					CallID:    "call-1",
					Name:      "lookup",
					Arguments: json.RawMessage(`{"key":"value"}`),
				}},
			},
			{
				Role: prompts.RoleTool,
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
	_, err := openai.ChatCompletionsMessages(prompts.Prompts{{
		Role:    prompts.RoleTool,
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

	client, err := openai.NewChatCompletions(horizon.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("test-key"),
	})
	require.NoError(t, err)

	// Execute one request and assert the normalized call ID, name, arguments,
	// and token usage.
	response, err := client.Generate(context.Background(), &horizon.Request{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.ToolCalls, 1)
	require.Equal(t, "call-1", response.ToolCalls[0].CallID)
	require.Equal(t, "lookup", response.ToolCalls[0].Name)
	require.JSONEq(t, `{"key":"value"}`, string(response.ToolCalls[0].Arguments))
	require.Equal(t, int64(5), response.Usage.TotalTokens)
}
*/

// Performs a generation, retrying on certain model failures, until all models
// are exhausted. This is intended to help with tests that are flaky due to
// failures that are unrelated to the test being performed.
func generateWithOpenRouterModels(
	t *testing.T,
	client provider.Generator,
	ctx context.Context,
	req *provider.Request,
	models []string,
) (*provider.Response, string) {
	t.Helper()

	var rep *provider.Response
	model := testenv.RunWithModels(t, models, func(model string) error {
		req.Model = model
		var err error
		rep, err = client.Generate(ctx, req)
		return testenv.ModelError(model, err)
	})
	return rep, model
}
