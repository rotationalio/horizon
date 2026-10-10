package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
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

		body, err := openai.ResponsesBody(context.Background(), req)
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

		body, err := openai.ResponsesBody(context.Background(), req)
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
				{Role: prompts.RoleUnknown, Content: "unknown"},
			},
		}

		_, err := openai.ResponsesBody(context.Background(), req)
		require.ErrorContains(t, err, "unsupported message role")
	})
}

// Verifies ordinary Horizon messages preserve their roles, phases, and content
// in Responses input items.
func TestResponsesMessages(t *testing.T) {
	items, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{
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
		parts, err := openai.ResponsesAttachments(context.Background(), attachments.Attachments{
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
		items, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{
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

		_, err := openai.ResponsesAttachments(ctx, attachments.Attachments{{
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

	client, err := openai.NewResponses(conf, provider.ResolveOptions())
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the responses API. Respond as quickly and as briefly as possible."},
				{Role: prompts.RoleUser, Content: "What is the capital of France?"},
			},
		}

		rep, model := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels(t))
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

// Verifies tool calls and results map to Responses input items and function
// tool definitions.
func TestResponsesToolMapping(t *testing.T) {
	messages, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{
		{Role: prompts.RoleUser, Content: "lookup"},
		{Role: prompts.RoleAssistant, ID: "fc-item-1", Status: "completed", ToolCalls: []prompts.ToolCall{{
			CallID: "call-1", Name: "lookup", Arguments: `{"key":"value"}`,
		}}},
		{Role: prompts.RoleTool, ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "42"}}},
	}, nil)

	require.NoError(t, err)
	require.Len(t, messages.OfInputItemList, 3)
	require.Equal(t, "lookup", messages.OfInputItemList[0].OfMessage.Content.OfInputItemContentList[0].OfInputText.Text)
	require.Equal(t, "fc-item-1", messages.OfInputItemList[1].OfFunctionCall.ID.Value)
	require.Equal(t, "call-1", messages.OfInputItemList[1].OfFunctionCall.CallID)
	require.Equal(t, "lookup", messages.OfInputItemList[1].OfFunctionCall.Name)
	require.Equal(t, `{"key":"value"}`, messages.OfInputItemList[1].OfFunctionCall.Arguments)
	require.Equal(t, responses.ResponseFunctionToolCallStatusCompleted, messages.OfInputItemList[1].OfFunctionCall.Status)
	require.Equal(t, "call-1", messages.OfInputItemList[2].OfFunctionCallOutput.CallID.Value)
	require.Equal(t, "42", messages.OfInputItemList[2].OfFunctionCallOutput.Output.OfString.Value)

	body, err := openai.ResponsesBody(context.Background(), &provider.Request{
		Model: "test-model",
		Tools: []capabilities.Definition{{
			Name:        "lookup",
			Description: "Look up a value.",
			InputSchema: &schema.Schema{
				Strict:   true,
				MimeType: mime.ApplicationSchemaJSON,
				Data:     `{"type":"object"}`,
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, body.Tools, 1)
	require.Equal(t, responses.ToolChoiceOptionsAuto, body.ToolChoice.OfToolChoiceMode.Value)
	require.True(t, body.ParallelToolCalls.Value)
	require.Equal(t, "lookup", body.Tools[0].OfFunction.Name)
	require.Equal(t, "Look up a value.", body.Tools[0].OfFunction.Description.Value)
	require.True(t, body.Tools[0].OfFunction.Strict.Value)
	require.Equal(t, "object", body.Tools[0].OfFunction.Parameters["type"])
}

// Both adapters share schema conversion, including the nil-schema default.
func TestToolSchemaValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *schema.Schema
		wantErr bool
	}{
		{
			name: "nil schema",
		},
		{
			name:   "object",
			schema: &schema.Schema{Data: `{"type":"object"}`, Strict: true},
		},
		{
			name:    "null",
			schema:  &schema.Schema{Data: `null`},
			wantErr: true,
		},
		{
			name:    "array",
			schema:  &schema.Schema{Data: `[]`},
			wantErr: true,
		},
		{
			name:    "boolean",
			schema:  &schema.Schema{Data: `true`},
			wantErr: true,
		},
		{
			name:    "malformed",
			schema:  &schema.Schema{Data: `{"type":`},
			wantErr: true,
		},
		{
			name:    "missing data",
			schema:  &schema.Schema{},
			wantErr: true,
		},
		{
			name:    "remote schema not implemented",
			schema:  &schema.Schema{URI: "https://example.com/schema.json"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: send the same schema through both real request converters.
			request := &provider.Request{Tools: []capabilities.Definition{{
				Name:        "lookup",
				InputSchema: tt.schema,
			}}}

			// Execute conversion without making any network requests.
			chat, chatErr := openai.ChatCompletionsBody(context.Background(), request)
			response, responseErr := openai.ResponsesBody(context.Background(), request)

			// Assert invalid schemas fail with tool context in both protocols.
			if tt.wantErr {
				require.ErrorContains(t, chatErr, `tool "lookup" input schema`)
				require.ErrorContains(t, responseErr, `tool "lookup" input schema`)
				return
			}
			require.NoError(t, chatErr)
			require.NoError(t, responseErr)

			// Valid schemas preserve object parameters and explicitly specify strict.
			strict := tt.schema != nil && tt.schema.Strict
			require.Len(t, chat.Tools, 1)
			require.Len(t, response.Tools, 1)
			require.Equal(t, "object", chat.Tools[0].OfFunction.Function.Parameters["type"])
			require.Equal(t, "object", response.Tools[0].OfFunction.Parameters["type"])
			require.Equal(t, strict, chat.Tools[0].OfFunction.Function.Strict.Value)
			require.Equal(t, strict, response.Tools[0].OfFunction.Strict.Value)
			require.True(t, chat.Tools[0].OfFunction.Function.Strict.Valid())
			require.True(t, response.Tools[0].OfFunction.Strict.Valid())
		})
	}
}

// Assistant replay preserves provider output identity and refusal semantics.
func TestResponsesAssistantReplay(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		promptType  prompts.Type
		meta        map[string]any
		wantText    string
		wantRefusal string
	}{
		{
			name:       "text",
			content:    "answer",
			promptType: prompts.TypeMessage,
			wantText:   "answer",
		},
		{
			name:        "refusal only",
			content:     "cannot answer",
			promptType:  prompts.TypeRefusal,
			wantRefusal: "cannot answer",
		},
		{
			name:        "text and refusal",
			content:     "partial answer",
			promptType:  prompts.TypeRefusal,
			meta:        map[string]any{"refusal": "cannot continue"},
			wantText:    "partial answer",
			wantRefusal: "cannot continue",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: replay an assistant output message with its original item identity.
			message := &prompts.Prompt{
				ID:      "msg-1",
				Role:    prompts.RoleAssistant,
				Type:    tt.promptType,
				Content: tt.content,
				Meta:    tt.meta,
				Status:  "completed",
				Phase:   "final_answer",
			}

			// Execute the public converter to exercise the extracted assistant helper.
			input, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{message}, nil)

			// Assert identity and content are preserved, not rebuilt as a generic message.
			require.NoError(t, err)
			require.Len(t, input.OfInputItemList, 1)
			item := input.OfInputItemList[0].OfOutputMessage
			require.NotNil(t, item)
			require.Equal(t, "msg-1", item.ID)
			require.Equal(t, responses.ResponseOutputMessageStatusCompleted, item.Status)
			require.Equal(t, responses.ResponseOutputMessagePhaseFinalAnswer, item.Phase)
			index := 0
			if tt.wantText != "" {
				require.Equal(t, tt.wantText, item.Content[index].OfOutputText.Text)
				index++
			}
			if tt.wantRefusal != "" {
				require.Equal(t, tt.wantRefusal, item.Content[index].OfRefusal.Refusal)
				index++
			}
			require.Len(t, item.Content, index)
		})
	}

	t.Run("invalid reasoning metadata", func(t *testing.T) {
		// Corrupted replay data must fail conversion rather than become assistant text.
		_, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{{
			Role: prompts.RoleAssistant,
			Type: prompts.TypeReasoning,
			Meta: map[string]any{"responses_reasoning_item": "{"},
		}}, nil)
		require.ErrorContains(t, err, "decode Responses reasoning item")
	})
}

// Verifies Responses output items preserve function-call IDs and reasoning
// context, and can be sent back as an ordered input transcript.
func TestResponsesGenerateMapsToolCalls(t *testing.T) {
	// Setup: the real adapter receives reasoning, a call, and commentary in order.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp-test",
			"object":"response",
			"created_at":1735689600,
			"status":"completed",
			"model":"test-model",
			"output":[
				{
					"type":"reasoning",
					"id":"rs-1",
					"summary":[{"type":"summary_text","text":"Looking up."},{"type":"summary_text","text":"Checking the tool."}],
					"encrypted_content":"opaque-reasoning",
					"status":"completed"
				},
				{
					"type":"function_call",
					"id":"fc-item-1",
					"call_id":"call-1",
					"name":"lookup",
					"arguments":"{\"key\":\"value\"}",
					"status":"completed"
				},
				{
					"type":"message",
					"id":"msg-1",
					"status":"completed",
					"role":"assistant",
					"phase":"commentary",
					"content":[{"type":"output_text","text":"Checking the result.","annotations":[]}]
				}
			],
			"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}
		}`))
	}))
	defer server.Close()

	client, err := openai.NewResponses(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("test-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)

	// Execute one inference against the local HTTP fixture.
	response, err := client.Generate(context.Background(), &provider.Request{Model: "test-model"})
	require.NoError(t, err)
	require.Len(t, response.Output, 3)
	require.Equal(t, "rs-1", response.Output[0].ID)
	require.Equal(t, "Looking up.\nChecking the tool.", response.Output[0].Content)
	require.Equal(t, prompts.TypeReasoning, response.Output[0].Type)
	require.Equal(t, "fc-item-1", response.Output[1].ID)
	require.Equal(t, "call-1", response.Output[1].ToolCalls[0].CallID)
	require.Equal(t, `{"key":"value"}`, response.Output[1].ToolCalls[0].Arguments)
	require.Equal(t, "msg-1", response.Output[2].ID)
	require.Equal(t, "commentary", response.Output[2].Phase)
	require.Equal(t, int64(5), response.Usage.TotalTokens)

	// Replay the full provider transcript, not the application's filtered history.
	transcript := append(prompts.Prompts(nil), response.Output...)
	transcript = append(transcript, &prompts.Prompt{
		Role:        prompts.RoleTool,
		ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "42"}},
	})
	items, err := openai.ResponsesMessages(context.Background(), transcript, nil)
	require.NoError(t, err)
	// Assert reasoning and both distinct IDs survive the request conversion.
	require.Len(t, items.OfInputItemList, 4)
	require.Equal(t, "rs-1", items.OfInputItemList[0].OfReasoning.ID)
	require.Equal(t, "opaque-reasoning", items.OfInputItemList[0].OfReasoning.EncryptedContent.Value)
	require.Equal(t, "fc-item-1", items.OfInputItemList[1].OfFunctionCall.ID.Value)
	require.Equal(t, "call-1", items.OfInputItemList[1].OfFunctionCall.CallID)
	require.Equal(t, "msg-1", items.OfInputItemList[2].OfOutputMessage.ID)
	require.Equal(t, responses.ResponseOutputMessagePhaseCommentary, items.OfInputItemList[2].OfOutputMessage.Phase)
	require.Equal(t, responses.ResponseOutputMessageStatusCompleted, items.OfInputItemList[2].OfOutputMessage.Status)
	require.Equal(t, "42", items.OfInputItemList[3].OfFunctionCallOutput.Output.OfString.Value)
}

// Verifies API-level failures and empty Responses outputs map to the shared sentinels.
func TestResponsesGenerateResponseErrors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError error
	}{
		{
			name:      "API error",
			body:      `{"id":"resp-error","object":"response","created_at":1,"status":"failed","model":"test-model","error":{"code":"invalid_prompt","message":"The prompt is invalid."},"output":[]}`,
			wantError: errors.ErrOpenAIResponsesError,
		},
		{
			name:      "no output",
			body:      `{"id":"resp-empty","object":"response","created_at":1,"status":"completed","model":"test-model","output":[]}`,
			wantError: errors.ErrNoResponseOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: serve a successful HTTP response containing the API-level case.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, err := openai.NewResponses(provider.Config{
				InferenceEndpoint: server.URL + "/",
				Credentials:       auth.NewAPIKey("test-key"),
			}, provider.ResolveOptions())
			require.NoError(t, err)

			// Execute: let the Responses SDK decode the fixture before adapter handling.
			_, err = client.Generate(context.Background(), &provider.Request{Model: "test-model"})

			// Assert: the conversion returns the expected sentinel.
			require.ErrorIs(t, err, tt.wantError)
		})
	}
}

// Verifies Responses text, refusal, logprob, citation, and status fields map to Horizon prompts.
func TestResponsesGenerateMapsMessageDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp-mapping",
			"object":"response",
			"created_at":1735689600,
			"status":"completed",
			"service_tier":"default",
			"model":"test-model",
			"output":[{
				"type":"message",
				"id":"msg-mapping",
				"status":"completed",
				"role":"assistant",
				"phase":"final_answer",
				"content":[
					{
						"type":"output_text",
						"text":"A partial answer.",
						"logprobs":[{"token":"A","logprob":-0.1,"bytes":[65],"top_logprobs":[]}],
						"annotations":[{"type":"url_citation","start_index":2,"end_index":8,"title":"Source","url":"https://example.com/source"}]
					},
					{"type":"refusal","refusal":"I cannot provide more."}
				]
			},{
				"type":"message",
				"id":"msg-refusal-only",
				"status":"completed",
				"role":"assistant",
				"content":[{"type":"refusal","refusal":"I cannot answer that."}]
			}],
			"usage":{"input_tokens":7,"output_tokens":4,"total_tokens":11,"cost":0.0125}
		}`))
	}))
	defer server.Close()

	client, err := openai.NewResponses(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("test-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)

	response, err := client.Generate(context.Background(), &provider.Request{Model: "test-model"})
	require.NoError(t, err)
	require.Equal(t, "resp-mapping", response.ID)
	require.Equal(t, "test-model", response.Model)
	require.Equal(t, int64(7), response.Usage.InputTokens)
	require.Equal(t, int64(4), response.Usage.OutputTokens)
	require.Equal(t, int64(11), response.Usage.TotalTokens)
	require.InDelta(t, 0.0125, response.Usage.APICost, 0.000001)
	require.Equal(t, "default", response.Meta["service_tier"])
	require.Len(t, response.Output, 2)

	message := response.Output[0]
	require.Equal(t, "msg-mapping", message.ID)
	require.Equal(t, prompts.RoleAssistant, message.Role)
	require.Equal(t, prompts.TypeRefusal, message.Type)
	require.Equal(t, "A partial answer.", message.Content)
	require.Equal(t, "I cannot provide more.", message.Meta["refusal"])
	require.Equal(t, "completed", message.Status)
	require.Equal(t, "final_answer", message.Phase)
	require.NotEmpty(t, message.Meta["logprobs"])
	require.Equal(t, []prompts.Citation{{
		StartIndex: 2,
		EndIndex:   8,
		Title:      "Source",
		URL:        "https://example.com/source",
	}}, message.Citations)

	refusalOnly := response.Output[1]
	require.Equal(t, "msg-refusal-only", refusalOnly.ID)
	require.Equal(t, prompts.TypeRefusal, refusalOnly.Type)
	require.Equal(t, "I cannot answer that.", refusalOnly.Content)
	require.NotContains(t, refusalOnly.Meta, "refusal")
}

// Checks each supported attachment representation in Responses content.
func TestResponsesAttachmentConversion(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		filename    string
		data        []byte
		wantText    string
		wantImage   string
		wantFile    string
	}{
		{
			name:        "text",
			contentType: "text/plain; charset=utf-8",
			data:        []byte("plain text"),
			wantText:    "plain text",
		},
		{
			name:        "image",
			contentType: string(mime.ImagePNG),
			data:        []byte("png"),
			wantImage:   "data:image/png;base64,cG5n",
		},
		{
			name:        "audio",
			contentType: string(mime.AudioMPEG),
			filename:    "audio.mp3",
			data:        []byte("audio"),
			wantFile:    "data:audio/mpeg;base64,YXVkaW8=",
		},
		{
			name:        "file",
			contentType: string(mime.ApplicationPDF),
			filename:    "report.pdf",
			data:        []byte("pdf"),
			wantFile:    "data:application/pdf;base64,cGRm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: convert one in-memory attachment to a Responses content part.
			parts, err := openai.ResponsesAttachments(context.Background(), attachments.Attachments{{
				ContentType: tt.contentType,
				Filename:    tt.filename,
				Data:        tt.data,
			}})

			// Assert: the selected SDK union variant contains the expected data.
			require.NoError(t, err)
			require.Len(t, parts, 1)
			switch {
			case tt.wantText != "":
				require.Equal(t, tt.wantText, parts[0].OfInputText.Text)
			case tt.wantImage != "":
				require.Equal(t, tt.wantImage, parts[0].OfInputImage.ImageURL.Value)
			case tt.wantFile != "":
				require.Equal(t, tt.wantFile, parts[0].OfInputFile.FileData.Value)
				require.Equal(t, tt.filename, parts[0].OfInputFile.Filename.Value)
			}
		})
	}
}

// Checks role mapping, user attachments, assistant calls, and tool results.
func TestResponsesPromptConversion(t *testing.T) {
	// Setup: include all message roles, one user attachment, and a function round trip.
	input := prompts.Prompts{
		{Role: prompts.RoleSystem, Content: "system"},
		{Role: prompts.RoleDeveloper, Content: "developer"},
		{Role: prompts.RoleUser, Content: "question", Phase: "commentary"},
		{Role: prompts.RoleAssistant, ID: "fc-item-1", Status: "completed", ToolCalls: []prompts.ToolCall{{
			CallID: "call-1", Name: "lookup", Arguments: `{"q":"x"}`,
		}}},
		{Role: prompts.RoleTool, ToolResults: []prompts.ToolResponse{{CallID: "call-1", Content: "answer"}}},
	}
	files := attachments.Attachments{{
		ContentType: string(mime.ImagePNG),
		Filename:    "image.png",
		Data:        []byte("png"),
	}}

	// Execute: convert the transcript and attachment to Responses input items.
	inputItems, err := openai.ResponsesMessages(context.Background(), input, files)

	// Assert: roles, phase, attachment position, and distinct function IDs survive.
	require.NoError(t, err)
	require.Len(t, inputItems.OfInputItemList, 5)
	require.Equal(t, responses.EasyInputMessageRoleSystem, inputItems.OfInputItemList[0].OfMessage.Role)
	require.Equal(t, responses.EasyInputMessageRoleDeveloper, inputItems.OfInputItemList[1].OfMessage.Role)
	userParts := inputItems.OfInputItemList[2].OfMessage.Content.OfInputItemContentList
	require.Len(t, userParts, 2)
	require.Equal(t, "question", userParts[0].OfInputText.Text)
	require.Equal(t, "data:image/png;base64,cG5n", userParts[1].OfInputImage.ImageURL.Value)
	require.Equal(t, responses.EasyInputMessagePhaseCommentary, inputItems.OfInputItemList[2].OfMessage.Phase)
	require.Equal(t, "fc-item-1", inputItems.OfInputItemList[3].OfFunctionCall.ID.Value)
	require.Equal(t, "call-1", inputItems.OfInputItemList[3].OfFunctionCall.CallID)
	require.Equal(t, "call-1", inputItems.OfInputItemList[4].OfFunctionCallOutput.CallID.Value)
	require.Equal(t, "answer", inputItems.OfInputItemList[4].OfFunctionCallOutput.Output.OfString.Value)
}

// Checks Responses output items map to Horizon reasoning, tool, and message prompts.
func TestResponsesOutputConversion(t *testing.T) {
	// Setup: return a reasoning item, function call, and message with citation and refusal.
	client := newResponsesFixtureClient(t, `{
		"id":"response-conversion",
		"object":"response",
		"created_at":1735689600,
		"status":"completed",
		"service_tier":"default",
		"model":"fixture-model",
		"output":[
			{"type":"reasoning","id":"rs-1","summary":[{"type":"summary_text","text":"Plan."},{"type":"summary_text","text":"Act."}],"encrypted_content":"opaque","status":"completed"},
			{"type":"function_call","id":"fc-item-1","call_id":"call-1","name":"lookup","arguments":"{}","status":"completed"},
			{"type":"message","id":"msg-1","status":"completed","role":"assistant","phase":"final_answer","content":[
				{"type":"output_text","text":"Partial answer.","logprobs":[{"token":"Partial","logprob":-0.1,"bytes":[80],"top_logprobs":[]}],"annotations":[{"type":"url_citation","start_index":0,"end_index":7,"title":"Source","url":"https://example.com"}]},
				{"type":"refusal","refusal":"Cannot continue."}
			]},
			{"type":"message","id":"msg-plain","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Plain answer.","annotations":[]}]},
			{"type":"message","id":"msg-refusal","status":"completed","role":"assistant","content":[{"type":"refusal","refusal":"Refusal only."}]}
		],
		"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7,"cost":0.02}
	}`)

	// Execute: decode the API response through the production Generate method.
	response, err := client.Generate(context.Background(), &provider.Request{Model: "fixture-model"})

	// Assert: provider-specific output data survives conversion to Horizon types.
	require.NoError(t, err)
	require.Equal(t, "response-conversion", response.ID)
	require.Equal(t, "fixture-model", response.Model)
	require.Equal(t, int64(4), response.Usage.InputTokens)
	require.Equal(t, int64(3), response.Usage.OutputTokens)
	require.Equal(t, int64(7), response.Usage.TotalTokens)
	require.InDelta(t, 0.02, response.Usage.APICost, 0.000001)
	require.Equal(t, "default", response.Meta["service_tier"])
	require.Len(t, response.Output, 5)
	require.Equal(t, "Plan.\nAct.", response.Output[0].Content)
	require.Equal(t, prompts.TypeReasoning, response.Output[0].Type)
	require.Equal(t, "fc-item-1", response.Output[1].ID)
	require.Equal(t, "call-1", response.Output[1].ToolCalls[0].CallID)
	require.Equal(t, "msg-1", response.Output[2].ID)
	require.Equal(t, prompts.TypeRefusal, response.Output[2].Type)
	require.Equal(t, "Partial answer.", response.Output[2].Content)
	require.Equal(t, "Cannot continue.", response.Output[2].Meta["refusal"])
	require.NotEmpty(t, response.Output[2].Meta["logprobs"])
	require.Equal(t, []prompts.Citation{{StartIndex: 0, EndIndex: 7, Title: "Source", URL: "https://example.com"}}, response.Output[2].Citations)
	require.Equal(t, "msg-plain", response.Output[3].ID)
	require.Equal(t, prompts.TypeMessage, response.Output[3].Type)
	require.Equal(t, "Plain answer.", response.Output[3].Content)
	require.Equal(t, prompts.TypeRefusal, response.Output[4].Type)
	require.Equal(t, "Refusal only.", response.Output[4].Content)
	require.NotContains(t, response.Output[4].Meta, "refusal")
}

// Checks that API and empty-output failures remain recognizable as sentinels.
func TestResponsesOutputConversionErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{
			name: "API error",
			body: `{"id":"api-error","object":"response","created_at":1,"status":"failed","model":"fixture-model","error":{"code":"invalid_prompt","message":"Invalid prompt."},"output":[]}`,
			want: errors.ErrOpenAIResponsesError,
		},
		{
			name: "empty output",
			body: `{"id":"empty","object":"response","created_at":1,"status":"completed","model":"fixture-model","output":[]}`,
			want: errors.ErrNoResponseOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: decode the fixture and convert the resulting response.
			client := newResponsesFixtureClient(t, tt.body)
			_, err := client.Generate(context.Background(), &provider.Request{Model: "fixture-model"})

			// Assert: the matching sentinel is returned for each failure mode.
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// Checks configured, default, and invalid request-body conversion cases.
func TestResponsesRequestBodyConversion(t *testing.T) {
	t.Run("configured options and tools", func(t *testing.T) {
		// Setup: populate supported options, a function tool, and a JSON output schema.
		request := &provider.Request{
			Model: "fixture-model",
			Input: prompts.Prompts{{Role: prompts.RoleUser, Content: "question"}},
			Tools: []capabilities.Definition{{
				Name:        "lookup",
				Description: "Find a value.",
				InputSchema: &schema.Schema{Strict: true, Data: `{"type":"object"}`},
			}},
			OutputSchema: &schema.Schema{
				Name:        "result",
				Description: "Structured result.",
				Strict:      true,
				MimeType:    mime.ApplicationSchemaJSON,
				Data:        `{"type":"object"}`,
			},
			Params: params.New(map[string]any{
				"temperature":            0.2,
				"max_output_tokens":      64,
				"top_p":                  0.9,
				"verbosity":              "low",
				"reasoning_effort":       "low",
				"reasoning_summary":      "auto",
				"top_logprobs":           2,
				"prompt_cache_key":       "cache-key",
				"safety_identifier":      "safety-id",
				"service_tier":           "default",
				"truncation":             "auto",
				"prompt_cache_retention": "24h",
			}),
		}

		// Execute: construct the Responses SDK request body.
		body, err := openai.ResponsesBody(context.Background(), request)

		// Assert: model, options, function tool, and structured format are preserved.
		require.NoError(t, err)
		require.Equal(t, "fixture-model", body.Model)
		require.False(t, body.Background.Value)
		require.False(t, body.Store.Value)
		require.Equal(t, int64(64), body.MaxOutputTokens.Value)
		require.InDelta(t, 0.2, body.Temperature.Value, 0.0001)
		require.InDelta(t, 0.9, body.TopP.Value, 0.0001)
		require.Equal(t, int64(2), body.TopLogprobs.Value)
		require.Equal(t, []responses.ResponseIncludable{responses.ResponseIncludableMessageOutputTextLogprobs}, body.Include)
		require.Equal(t, "cache-key", body.PromptCacheKey.Value)
		require.Equal(t, "safety-id", body.SafetyIdentifier.Value)
		require.Equal(t, "low", string(body.Text.Verbosity))
		require.Equal(t, "low", string(body.Reasoning.Effort))
		require.Equal(t, "auto", string(body.Reasoning.Summary))
		require.Equal(t, "24h", string(body.PromptCacheRetention))
		require.Equal(t, "default", string(body.ServiceTier))
		require.Equal(t, responses.ResponseNewParamsTruncationAuto, body.Truncation)
		require.Equal(t, responses.ToolChoiceOptionsAuto, body.ToolChoice.OfToolChoiceMode.Value)
		require.True(t, body.ParallelToolCalls.Value)
		require.Equal(t, "lookup", body.Tools[0].OfFunction.Name)
		require.True(t, body.Tools[0].OfFunction.Strict.Value)
		require.Equal(t, "object", body.Tools[0].OfFunction.Parameters["type"])
		require.NotNil(t, body.Text.Format.OfJSONSchema)
		require.Equal(t, "result", body.Text.Format.OfJSONSchema.Name)
		require.Equal(t, "object", body.Text.Format.OfJSONSchema.Schema["type"])
	})

	t.Run("defaults without optional settings", func(t *testing.T) {
		// Execute: build a basic request with no tools, schemas, or parameters.
		body, err := openai.ResponsesBody(context.Background(), &provider.Request{Model: "fixture-model"})

		// Assert: required defaults are present without inventing optional values.
		require.NoError(t, err)
		require.Equal(t, "fixture-model", body.Model)
		require.Empty(t, body.Tools)
		require.Empty(t, body.Include)
		require.True(t, param.IsOmitted(body.MaxOutputTokens))
		require.NotNil(t, body.Text.Format.OfText)
	})

	t.Run("invalid tool schema", func(t *testing.T) {
		// Execute: invalid tool parameters should retain their tool context.
		_, err := openai.ResponsesBody(context.Background(), &provider.Request{Tools: []capabilities.Definition{{
			Name: "lookup", InputSchema: &schema.Schema{Data: `null`},
		}}})

		// Assert: the error wraps the shared schema sentinel.
		require.ErrorContains(t, err, `tool "lookup" input schema`)
		require.ErrorIs(t, err, errors.ErrInvalidJSONSchema)
	})
}

// Checks each supported Responses output-format variant and its errors.
func TestResponsesFormatConversion(t *testing.T) {
	formats := []struct {
		name   string
		input  *schema.Schema
		format string
		want   error
	}{
		{
			name:   "nil defaults to text",
			format: "text",
		},
		{
			name:   "unknown defaults to text",
			input:  &schema.Schema{},
			format: "text",
		},
		{
			name:   "plain text",
			input:  &schema.Schema{MimeType: mime.TextPlain},
			format: "text",
		},
		{
			name:   "JSON object",
			input:  &schema.Schema{MimeType: mime.ApplicationJSON},
			format: "json_object",
		},
		{
			name:   "JSON schema",
			input:  &schema.Schema{MimeType: mime.ApplicationSchemaJSON, Data: `{"type":"object"}`},
			format: "json_schema",
		},
		{
			name:  "invalid schema",
			input: &schema.Schema{MimeType: mime.ApplicationSchemaJSON, Data: `null`},
			want:  errors.ErrInvalidJSONSchema,
		},
		{
			name:  "unsupported",
			input: &schema.Schema{MimeType: mime.TextHTML},
			want:  errors.ErrUnsupportedOutputSchemaMimeType,
		},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: convert the schema to a Responses output format.
			format, err := openai.ResponsesResponseFormat(tt.input)

			// Assert: the expected SDK variant is populated, or the error is wrapped.
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			switch tt.format {
			case "text":
				require.NotNil(t, format.OfText)
			case "json_object":
				require.NotNil(t, format.OfJSONObject)
			case "json_schema":
				require.NotNil(t, format.OfJSONSchema)
				require.Equal(t, "object", format.OfJSONSchema.Schema["type"])
			}
		})
	}
}

// Checks completed, incomplete, content-filtered, and failed Responses statuses.
func TestResponsesStatusConversion(t *testing.T) {
	statuses := []struct {
		status responses.ResponseStatus
		reason string
		want   prompts.Type
	}{
		{status: responses.ResponseStatusCompleted, want: prompts.TypeMessage},
		{status: responses.ResponseStatusIncomplete, reason: "max_output_tokens", want: prompts.TypeMessage},
		{status: responses.ResponseStatusIncomplete, reason: "content_filter", want: prompts.TypeContentFilter},
		{status: responses.ResponseStatusFailed, want: prompts.TypeUnknown},
	}

	for _, tt := range statuses {
		t.Run(string(tt.status)+"/"+tt.reason, func(t *testing.T) {
			// Execute and assert: response status and reason map to a prompt type.
			require.Equal(t, tt.want, openai.ResponseStatusType(tt.status, tt.reason))
		})
	}
}

// Checks assistant transcript replay for plain messages, refusals, calls, and reasoning.
func TestResponsesAssistantTranscriptConversion(t *testing.T) {
	tests := []struct {
		name        string
		message     *prompts.Prompt
		wantCount   int
		wantText    string
		wantRefusal string
		wantReason  bool
		wantError   bool
	}{
		{
			name:      "caller-authored assistant message",
			message:   &prompts.Prompt{Role: prompts.RoleAssistant, Content: "hello", Phase: "final_answer"},
			wantCount: 1,
			wantText:  "hello",
		},
		{
			name: "provider output plain text",
			message: &prompts.Prompt{ID: "msg-plain", Role: prompts.RoleAssistant, Type: prompts.TypeMessage,
				Content: "plain response", Status: "completed"},
			wantCount: 1,
			wantText:  "plain response",
		},
		{
			name: "provider output with refusal only",
			message: &prompts.Prompt{ID: "msg-1", Role: prompts.RoleAssistant, Type: prompts.TypeRefusal,
				Content: "cannot answer", Status: "completed", Phase: "final_answer"},
			wantCount:   1,
			wantRefusal: "cannot answer",
		},
		{
			name: "provider output with text and refusal",
			message: &prompts.Prompt{ID: "msg-2", Role: prompts.RoleAssistant, Type: prompts.TypeRefusal,
				Content: "partial", Meta: map[string]any{"refusal": "cannot continue"}},
			wantCount:   1,
			wantText:    "partial",
			wantRefusal: "cannot continue",
		},
		{
			name: "reasoning replay",
			message: &prompts.Prompt{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning,
				Meta: map[string]any{"responses_reasoning_item": `{"type":"reasoning","id":"rs-1","summary":[],"encrypted_content":"opaque"}`}},
			wantCount:  1,
			wantReason: true,
		},
		{
			name:      "invalid reasoning metadata",
			message:   &prompts.Prompt{Role: prompts.RoleAssistant, Type: prompts.TypeReasoning, Meta: map[string]any{"responses_reasoning_item": "{"}},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: replay one assistant prompt through the public message converter.
			input, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{tt.message}, nil)

			// Assert: replay preserves the provider item variant and its content.
			if tt.wantError {
				require.ErrorContains(t, err, "decode Responses reasoning item")
				return
			}
			require.NoError(t, err)
			require.Len(t, input.OfInputItemList, tt.wantCount)
			if tt.wantReason {
				require.Equal(t, "rs-1", input.OfInputItemList[0].OfReasoning.ID)
				require.Equal(t, "opaque", input.OfInputItemList[0].OfReasoning.EncryptedContent.Value)
				return
			}
			if tt.message.ID != "" {
				output := input.OfInputItemList[0].OfOutputMessage
				if tt.wantText != "" {
					require.Equal(t, tt.wantText, output.Content[0].OfOutputText.Text)
				}
				if tt.wantRefusal != "" {
					index := 0
					if tt.wantText != "" {
						index = 1
					}
					require.Equal(t, tt.wantRefusal, output.Content[index].OfRefusal.Refusal)
				}
				return
			}
			require.Equal(t, tt.wantText, input.OfInputItemList[0].OfMessage.Content.OfString.Value)
		})
	}
}

// Checks missing tool results and unknown prompt roles fail Responses conversion.
func TestResponsesPromptConversionErrors(t *testing.T) {
	_, err := openai.ResponsesMessages(context.Background(), prompts.Prompts{{Role: prompts.RoleTool}}, nil)
	require.ErrorIs(t, err, errors.ErrToolMessageMissingResults)

	_, err = openai.ResponsesMessages(context.Background(), prompts.Prompts{{Role: prompts.RoleUnknown}}, nil)
	require.ErrorIs(t, err, errors.ErrUnsupportedMessageRole)
}

// newResponsesFixtureClient serves a deterministic Responses payload through a test server.
func newResponsesFixtureClient(t *testing.T, body string) *openai.ResponsesClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := openai.NewResponses(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("fixture-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)
	return client
}
