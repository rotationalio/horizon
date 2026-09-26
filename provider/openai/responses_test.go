package openai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// Verifies request parameters, output formats, and invalid message roles are
// mapped correctly for the Responses API.
func TestResponsesBody(t *testing.T) {
	t.Run("MessagesAndParams", func(t *testing.T) {
		req := &provider.Request{
			Model: "gpt-4.1-mini",
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "Be brief."},
				{Role: prompts.RoleUser, Content: "What is the capital of France?"},
			},
			Params: params.New(map[string]any{
				"temperature":       0.2,
				"max_output_tokens": 64,
				"top_p":             0.9,
				"verbosity":         "low",
				"reasoning_effort":  "low",
				"reasoning_summary": "auto",
				"top_logprobs":      2,
				"prompt_cache_key":  "cache-key",
				"safety_identifier": "safety-id",
				"service_tier":      "default",
				"truncation":        "auto",
			}),
		}

		body, err := openai.ResponsesBody(req)
		require.NoError(t, err)
		require.Equal(t, "gpt-4.1-mini", body.Model)
		require.False(t, body.Background.Value)
		require.False(t, body.Store.Value)
		require.Equal(t, int64(64), body.MaxOutputTokens.Value)
		require.InDelta(t, 0.2, body.Temperature.Value, 0.0001)
		require.InDelta(t, 0.9, body.TopP.Value, 0.0001)
		require.Equal(t, int64(2), body.TopLogprobs.Value)
		require.Equal(t, []responses.ResponseIncludable{responses.ResponseIncludableMessageOutputTextLogprobs}, body.Include)
		require.Equal(t, "cache-key", body.PromptCacheKey.Value)
		require.Equal(t, "safety-id", body.SafetyIdentifier.Value)
		require.Equal(t, responses.ResponseTextConfigVerbosity("low"), body.Text.Verbosity)
		require.Equal(t, "low", string(body.Reasoning.Effort))
		require.Equal(t, "auto", string(body.Reasoning.Summary))
		require.Len(t, body.Input.OfInputItemList, 2)
		require.Equal(t, responses.EasyInputMessageRoleDeveloper, body.Input.OfInputItemList[0].OfMessage.Role)
		require.Equal(t, "Be brief.", body.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
		require.Equal(t, responses.EasyInputMessageRoleUser, body.Input.OfInputItemList[1].OfMessage.Role)
		require.NotNil(t, body.Text.Format.OfText)
	})

	t.Run("JSONSchema", func(t *testing.T) {
		req := &provider.Request{
			Model: "gpt-4.1-mini",
			Input: prompts.Prompts{
				{Role: prompts.RoleUser, Content: "Return colors."},
			},
			OutputSchema: &schema.Schema{
				Name:        "colors",
				Description: "A list of colors",
				Strict:      true,
				MimeType:    mime.ApplicationSchemaJSON,
				Data:        `{"type":"object","properties":{"colors":{"type":"array","items":{"type":"string"}}},"required":["colors"]}`,
			},
		}

		body, err := openai.ResponsesBody(req)
		require.NoError(t, err)
		require.NotNil(t, body.Text.Format.OfJSONSchema)
		require.Equal(t, "colors", body.Text.Format.OfJSONSchema.Name)
		require.True(t, body.Text.Format.OfJSONSchema.Strict.Value)
		require.Equal(t, "A list of colors", body.Text.Format.OfJSONSchema.Description.Value)
		require.Equal(t, "object", body.Text.Format.OfJSONSchema.Schema["type"])
	})

	t.Run("UnsupportedRole", func(t *testing.T) {
		req := &provider.Request{
			Model: "gpt-4.1-mini",
			Input: prompts.Prompts{
				{Role: prompts.RoleTool, Content: "tool result"},
			},
		}

		_, err := openai.ResponsesBody(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "tool messages are not supported yet")
	})
}

// Verifies ordinary Horizon messages preserve their roles, phases, and content
// in Responses input items.
func TestResponsesMessages(t *testing.T) {
	items, err := openai.ResponsesMessages(prompts.Prompts{
		{Role: prompts.RoleSystem, Content: "You are a helper."},
		{Role: prompts.RoleAssistant, Content: "Hello.", Phase: "final_answer"},
		{Role: prompts.RoleUser, Content: "Hi."},
	}, nil)
	require.NoError(t, err)
	require.Len(t, items.OfInputItemList, 3)
	require.Equal(t, responses.EasyInputMessageRoleSystem, items.OfInputItemList[0].OfMessage.Role)
	require.False(t, param.IsOmitted(items.OfInputItemList[0].OfMessage.Content.OfString))
	require.Equal(t, responses.EasyInputMessagePhaseFinalAnswer, items.OfInputItemList[1].OfMessage.Phase)
	require.Len(t, items.OfInputItemList[2].OfMessage.Content.OfInputItemContentList, 1)
	require.Equal(t, "Hi.", items.OfInputItemList[2].OfMessage.Content.OfInputItemContentList[0].OfInputText.Text)
}

// Verifies supported attachment types become the corresponding Responses input
// content parts.
func TestResponsesAttachments(t *testing.T) {
	t.Run("Image", func(t *testing.T) {
		parts, err := openai.ResponsesAttachments(attachments.Attachments{
			{
				ContentType: string(mime.ImagePNG),
				Filename:    "image.png",
				Data:        []byte("png"),
			},
		})
		require.NoError(t, err)
		require.Len(t, parts, 1)
		require.NotNil(t, parts[0].OfInputImage)
		require.Equal(t, responses.ResponseInputImageDetailAuto, parts[0].OfInputImage.Detail)
		require.Equal(t, "data:image/png;base64,cG5n", parts[0].OfInputImage.ImageURL.Value)
	})

	t.Run("WithUserMessage", func(t *testing.T) {
		items, err := openai.ResponsesMessages(prompts.Prompts{
			{Role: prompts.RoleDeveloper, Content: "Be brief."},
			{Role: prompts.RoleUser, Content: "What is in the image?"},
		}, attachments.Attachments{
			{
				ContentType: string(mime.ImagePNG),
				Filename:    "image.png",
				Data:        []byte("png"),
			},
		})
		require.NoError(t, err)
		require.False(t, param.IsOmitted(items.OfInputItemList[0].OfMessage.Content.OfString))
		userParts := items.OfInputItemList[1].OfMessage.Content.OfInputItemContentList
		require.Len(t, userParts, 2)
		require.Equal(t, "What is in the image?", userParts[0].OfInputText.Text)
		require.NotNil(t, userParts[1].OfInputImage)
		require.Equal(t, "data:image/png;base64,cG5n", userParts[1].OfInputImage.ImageURL.Value)
	})

	t.Run("CanceledDownload", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := openai.ResponsesAttachmentsContext(ctx, attachments.Attachments{{
			ContentType: string(mime.ApplicationPDF),
			Filename:    "document.pdf",
			URL:         "https://example.com/document.pdf",
		}})
		require.ErrorIs(t, err, context.Canceled)
	})
}

// Verifies Horizon output schemas map to Responses response format variants.
func TestResponsesResponseFormat(t *testing.T) {
	t.Run("PlainText", func(t *testing.T) {
		format, err := openai.ResponsesResponseFormat(nil)
		require.NoError(t, err)
		require.NotNil(t, format.OfText)
	})

	t.Run("JSONObject", func(t *testing.T) {
		format, err := openai.ResponsesResponseFormat(&schema.Schema{MimeType: mime.ApplicationJSON})
		require.NoError(t, err)
		require.NotNil(t, format.OfJSONObject)
	})

	t.Run("Unsupported", func(t *testing.T) {
		_, err := openai.ResponsesResponseFormat(&schema.Schema{MimeType: mime.TextHTML})
		require.Error(t, err)
	})
}

// Verifies Responses statuses map to Horizon output types.
func TestResponseStatusType(t *testing.T) {
	require.Equal(t, prompts.TypeMessage, openai.ResponseStatusType(responses.ResponseStatusCompleted, ""))
	require.Equal(t, prompts.TypeMessage, openai.ResponseStatusType(responses.ResponseStatusIncomplete, "max_output_tokens"))
	require.Equal(t, prompts.TypeContentFilter, openai.ResponseStatusType(responses.ResponseStatusIncomplete, "content_filter"))
	require.Equal(t, prompts.TypeUnknown, openai.ResponseStatusType(responses.ResponseStatusFailed, ""))
}

// Exercises the Responses endpoint against a live OpenRouter model.
func TestResponsesIntegration(t *testing.T) {
	// This test uses an open router SDK key to test the responses API.
	// This is an integration test, so use -short locally to skip the test.
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	conf := provider.Config{
		Credentials:       auth.NewAPIKey(testenv.OpenRouterAPIKey(t)),
		InferenceEndpoint: testenv.OpenRouterEndpointURL(t),
	}

	client, err := openai.NewResponses(conf)
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)
		output := rep.Output[len(rep.Output)-1]

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
				{Role: prompts.RoleSystem, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)

		output := rep.Output[len(rep.Output)-1]
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
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)

		output := rep.Output[len(rep.Output)-1]
		require.NotEmpty(t, output.Content)

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
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)

		output := rep.Output[len(rep.Output)-1]
		require.NotEmpty(t, output.Content)

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
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)

		output := rep.Output[len(rep.Output)-1]
		require.NotEmpty(t, output.Content)

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
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
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
		require.GreaterOrEqual(t, len(rep.Output), 1)

		output := rep.Output[len(rep.Output)-1]
		require.NotEmpty(t, output.Content)

		var topic map[string]any
		err = json.Unmarshal([]byte(output.Content), &topic)
		require.NoError(t, err)
		require.Contains(t, topic["topic"], "Bitcoin")
	})
}

// TODO: Re-enable the tool mapping tests once prompts expose the
// provider-neutral tool model.
// Verifies tool calls and results map to Responses input items and function
// tool definitions.
/*
func TestResponsesToolMapping(t *testing.T) {
	// Set up an assistant function call followed by its tool result.
	// Verify the input item sequence and the function tool request body.
	messages, err := openai.ResponsesMessages(prompts.Prompts{
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
	}, nil)

	require.NoError(t, err)
	require.Len(t, messages.OfInputItemList, 3)
	require.Equal(t, "lookup", messages.OfInputItemList[0].OfMessage.Content.OfInputItemContentList[0].OfInputText.Text)
	require.Equal(t, "call-1", messages.OfInputItemList[1].OfFunctionCall.CallID)
	require.Equal(t, "lookup", messages.OfInputItemList[1].OfFunctionCall.Name)
	require.Equal(t, openaisdk.String("call-1"), messages.OfInputItemList[2].OfFunctionCallOutput.CallID)
	require.Equal(t, "42", messages.OfInputItemList[2].OfFunctionCallOutput.Output.OfString.Value)

	body, err := openai.ResponsesBody(&horizon.Request{
		Model: "test-model",
		Tools: []capabilities.ToolDefinition{{
			Name:        "lookup",
			Description: "Look up a value.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})
	require.NoError(t, err)
	require.Len(t, body.Tools, 1)
	require.Equal(t, responses.ToolChoiceOptionsAuto, body.ToolChoice.OfToolChoiceMode.Value)
	require.True(t, body.ParallelToolCalls.Value)
	require.Equal(t, "lookup", body.Tools[0].OfFunction.Name)
	require.Equal(t, "Look up a value.", body.Tools[0].OfFunction.Description.Value)
	require.Equal(t, "object", body.Tools[0].OfFunction.Parameters["type"])
}

// Verifies a Responses function call is converted into a provider-neutral
// Horizon tool call.
func TestResponsesGenerateMapsToolCalls(t *testing.T) {
	// Return a minimal function-call response from a local server to exercise
	// SDK decoding without depending on a remote service.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp-test",
			"object":"response",
			"created_at":1735689600,
			"status":"completed",
			"model":"test-model",
			"output":[{
				"type":"function_call",
				"id":"fc-1",
				"call_id":"call-1",
				"name":"lookup",
				"arguments":"{\"key\":\"value\"}",
				"status":"completed"
			}],
			"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}
		}`))
	}))
	defer server.Close()

	client, err := openai.NewResponses(config.Provider{
		InferenceEndpoint: server.URL + "/",
		Credentials:       credtest.APIKey("test-key"),
	})
	require.NoError(t, err)

	// Execute one request and assert normalized call metadata and usage.
	response, err := client.Generate(context.Background(), &horizon.Request{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.ToolCalls, 1)
	require.Equal(t, "call-1", response.ToolCalls[0].CallID)
	require.Equal(t, "lookup", response.ToolCalls[0].Name)
	require.JSONEq(t, `{"key":"value"}`, string(response.ToolCalls[0].Arguments))
	require.Equal(t, int64(5), response.Usage.TotalTokens)
}
*/
