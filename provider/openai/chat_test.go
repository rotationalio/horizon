package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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

	client, err := openai.NewChatCompletions(conf, provider.ResolveOptions())
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
		defer cancel()

		req := &provider.Request{
			Input: prompts.Prompts{
				{Role: prompts.RoleDeveloper, Content: "This is a test of the chat completions API. Respond as quickly and as briefly as possible."},
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
		require.Len(t, rep.Output, 1)
		output := rep.Output[0]

		require.Empty(t, output.ID)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterMultimodalModels(t))
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
		ctx, cancel := context.WithTimeout(context.Background(), testenv.LiveRequestTimeout)
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

		rep, _ := generateWithOpenRouterModels(t, client, ctx, req, testenv.OpenRouterTextModels(t))
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
			parts, err := openai.ChatCompletionsAttachments(context.Background(), attachments.Attachments{{
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
		_, err := openai.ChatCompletionsAttachments(context.Background(), attachments.Attachments{{
			ContentType: "audio/ogg",
			Filename:    "sample.ogg",
			Data:        []byte("ogg"),
		}})
		require.ErrorContains(t, err, "unsupported chat completions audio content type")
	})
}

// Verifies cancellation is propagated while resolving Chat Completions
// attachments.
func TestChatCompletionsAttachmentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := openai.ChatCompletionsAttachments(ctx, attachments.Attachments{{
		ContentType: string(mime.ImagePNG),
		Filename:    "image.png",
		URL:         "https://example.com/image.png",
	}})
	require.ErrorIs(t, err, context.Canceled)
}

// Verifies tool definitions and tool transcripts map to the Chat Completions
// request format.
func TestChatToolMapping(t *testing.T) {
	body, err := openai.ChatCompletionsBody(context.Background(), &provider.Request{
		Model: "test-model",
		Input: prompts.Prompts{
			{Role: prompts.RoleUser, Content: "lookup"},
			{Role: prompts.RoleAssistant, ToolCalls: []prompts.ToolCall{{
				CallID: "call-1", Name: "lookup", Arguments: `{"key":"value"}`,
			}}},
			{Role: prompts.RoleTool, ToolResults: []prompts.ToolResponse{{
				CallID: "call-1", Content: "42",
			}}},
		},
		Tools: []capabilities.Definition{{
			Name:        "lookup",
			Description: "Look up a value.",
			InputSchema: &schema.Schema{
				Strict:   true,
				MimeType: mime.ApplicationSchemaJSON,
				Data:     `{"type":"object","properties":{"key":{"type":"string"}}}`,
			},
		}},
	})

	require.NoError(t, err)
	require.Len(t, body.Tools, 1)
	require.Equal(t, "auto", body.ToolChoice.OfAuto.Value)
	require.True(t, body.ParallelToolCalls.Value)
	require.Equal(t, "lookup", body.Tools[0].OfFunction.Function.Name)
	require.Equal(t, "Look up a value.", body.Tools[0].OfFunction.Function.Description.Value)
	require.True(t, body.Tools[0].OfFunction.Function.Strict.Value)
	require.Equal(t, "object", body.Tools[0].OfFunction.Function.Parameters["type"])
	require.Len(t, body.Messages, 3)
	require.Len(t, body.Messages[1].OfAssistant.ToolCalls, 1)
	require.Equal(t, "call-1", body.Messages[1].OfAssistant.ToolCalls[0].OfFunction.ID)
	require.Equal(t, `{"key":"value"}`, body.Messages[1].OfAssistant.ToolCalls[0].OfFunction.Function.Arguments)
	require.Equal(t, "call-1", body.Messages[2].OfTool.ToolCallID)
	require.Equal(t, "42", body.Messages[2].OfTool.Content.OfString.Value)
}

// Verifies invalid tool schemas fail and missing schemas default to an object.
func TestChatToolSchemaValidation(t *testing.T) {
	_, err := openai.ChatCompletionsBody(context.Background(), &provider.Request{Tools: []capabilities.Definition{{
		Name: "lookup",
		InputSchema: &schema.Schema{
			MimeType: mime.ApplicationSchemaJSON,
			Data:     `[]`,
		},
	}}})
	require.ErrorContains(t, err, `tool "lookup" input schema`)

	body, err := openai.ChatCompletionsBody(context.Background(), &provider.Request{Tools: []capabilities.Definition{{Name: "no_args"}}})
	require.NoError(t, err)
	require.Equal(t, "object", body.Tools[0].OfFunction.Function.Parameters["type"])
	require.False(t, body.Tools[0].OfFunction.Function.Strict.Value)
	require.True(t, body.Tools[0].OfFunction.Function.Strict.Valid())
}

// A tool message must include a result and its call ID.
func TestChatToolMessageRequiresResult(t *testing.T) {
	_, err := openai.ChatCompletionsMessages(context.Background(), prompts.Prompts{{Role: prompts.RoleTool}}, nil)
	require.ErrorIs(t, err, errors.ErrToolMessageMissingResults)
}

// Verifies a Chat Completions function call is converted into a provider-neutral
// prompt by the SDK response decoding path.
func TestChatGenerateMapsToolCalls(t *testing.T) {
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

	client, err := openai.NewChatCompletions(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("test-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)

	response, err := client.Generate(context.Background(), &provider.Request{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.Output, 1)
	require.Len(t, response.Output[0].ToolCalls, 1)
	require.Equal(t, prompts.TypeToolCall, response.Output[0].Type)
	require.Equal(t, "call-1", response.Output[0].ToolCalls[0].CallID)
	require.Equal(t, "lookup", response.Output[0].ToolCalls[0].Name)
	require.JSONEq(t, `{"key":"value"}`, response.Output[0].ToolCalls[0].Arguments)
	require.Equal(t, int64(5), response.Usage.TotalTokens)
}

// Verifies Chat Completions refusal and citation data maps to Horizon prompts.
func TestChatGenerateMapsRefusalAndCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-mapping",
			"object":"chat.completion",
			"created":1735689600,
			"model":"test-model",
			"service_tier":"default",
			"choices":[{
				"index":2,
				"finish_reason":"stop",
				"message":{
					"role":"assistant",
					"content":"A partial answer.",
					"refusal":"I cannot provide more.",
					"annotations":[{
						"type":"url_citation",
						"url_citation":{"start_index":2,"end_index":8,"title":"Source","url":"https://example.com/source"}
					}]
				}
			}],
			"usage":{"prompt_tokens":7,"completion_tokens":4,"total_tokens":11}
		}`))
	}))
	defer server.Close()

	client, err := openai.NewChatCompletions(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("test-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)

	response, err := client.Generate(context.Background(), &provider.Request{Model: "test-model"})
	require.NoError(t, err)
	require.Equal(t, "chatcmpl-mapping", response.ID)
	require.Equal(t, "test-model", response.Model)
	require.Equal(t, int64(7), response.Usage.InputTokens)
	require.Equal(t, int64(4), response.Usage.OutputTokens)
	require.Equal(t, int64(11), response.Usage.TotalTokens)
	require.Len(t, response.Output, 1)

	message := response.Output[0]
	require.Equal(t, prompts.RoleAssistant, message.Role)
	require.Equal(t, prompts.TypeRefusal, message.Type)
	require.Equal(t, "A partial answer.", message.Content)
	require.Equal(t, "I cannot provide more.", message.Meta["refusal"])
	require.Equal(t, "stop", message.Meta["finish_reason"])
	require.Equal(t, int64(2), message.Meta["index"])
	require.Equal(t, "default", response.Meta["service_tier"])
	require.Equal(t, []prompts.Citation{{
		StartIndex: 2,
		EndIndex:   8,
		Title:      "Source",
		URL:        "https://example.com/source",
	}}, message.Citations)
}

// Checks edge cases in Chat Completions response conversion, including refusals, costs, and unsupported tool types.
func TestChatGenerateMapsResponseEdgeCases(t *testing.T) {
	// Setup: each case uses a deterministic API response through the real SDK decoder.
	tests := []struct {
		name       string
		body       string
		wantError  error
		wantCost   float64
		wantText   string
		wantRefuse bool
	}{
		{
			name:      "no choices",
			body:      `{"id":"empty","object":"chat.completion","created":1,"model":"test-model","choices":[]}`,
			wantError: errors.ErrNoChatCompletions,
		},
		{
			name:       "cost and refusal only",
			body:       `{"id":"cost","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":null,"refusal":"I cannot help."}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"cost":0.0125}}`,
			wantCost:   0.0125,
			wantText:   "I cannot help.",
			wantRefuse: true,
		},
		{
			name:      "invalid cost",
			body:      `{"id":"bad-cost","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"cost":"invalid"}}`,
			wantError: strconv.ErrSyntax,
		},
		{
			name:      "unsupported tool call type",
			body:      `{"id":"bad-tool","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"computer","function":{"name":"run","arguments":"{}"}}]}}]}`,
			wantError: errors.ErrUnsupportedChatToolCallType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: serve the case's response body to the OpenAI client.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, err := openai.NewChatCompletions(provider.Config{
				InferenceEndpoint: server.URL + "/",
				Credentials:       auth.NewAPIKey("test-key"),
			}, provider.ResolveOptions())
			require.NoError(t, err)

			// Execute: decode the fixture through the production Generate method.
			response, err := client.Generate(context.Background(), &provider.Request{Model: "test-model"})

			// Assert: verify the selected mapping result or sentinel/wrapped failure.
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, tt.wantCost, response.Usage.APICost, 0.000001)
			require.Len(t, response.Output, 1)
			require.Equal(t, tt.wantText, response.Output[0].Content)
			require.Equal(t, tt.wantRefuse, response.Output[0].Type == prompts.TypeRefusal)
		})
	}
}

// Checks each supported attachment representation in Chat Completions content.
func TestChatAttachmentConversion(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		filename    string
		data        []byte
		wantText    string
		wantImage   string
		wantAudio   string
		wantFormat  string
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
			data:        []byte("audio"),
			wantAudio:   "YXVkaW8=",
			wantFormat:  "mp3",
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
			// Execute: convert one in-memory attachment to a Chat Completions part.
			parts, err := openai.ChatCompletionsAttachments(context.Background(), attachments.Attachments{{
				ContentType: tt.contentType,
				Filename:    tt.filename,
				Data:        tt.data,
			}})

			// Assert: exactly the expected SDK content variant is populated.
			require.NoError(t, err)
			require.Len(t, parts, 1)
			switch {
			case tt.wantText != "":
				require.Equal(t, tt.wantText, parts[0].OfText.Text)
			case tt.wantImage != "":
				require.Equal(t, tt.wantImage, parts[0].OfImageURL.ImageURL.URL)
			case tt.wantAudio != "":
				require.Equal(t, tt.wantAudio, parts[0].OfInputAudio.InputAudio.Data)
				require.Equal(t, tt.wantFormat, parts[0].OfInputAudio.InputAudio.Format)
			case tt.wantFile != "":
				require.Equal(t, tt.wantFile, parts[0].OfFile.File.FileData.Value)
				require.Equal(t, tt.filename, parts[0].OfFile.File.Filename.Value)
			}
		})
	}
}

// Checks role mapping, tool-call mapping, and one-to-many tool-result mapping.
func TestChatPromptConversion(t *testing.T) {
	// Setup: include every supported role and two results on one tool prompt.
	input := prompts.Prompts{
		{Role: prompts.RoleUser, Content: "question"},
		{Role: prompts.RoleAssistant, Content: "answer", ToolCalls: []prompts.ToolCall{{
			CallID: "call-1", Name: "lookup", Arguments: `{"q":"x"}`,
		}}},
		{Role: prompts.RoleSystem, Content: "system"},
		{Role: prompts.RoleDeveloper, Content: "developer"},
		{Role: prompts.RoleTool, ToolResults: []prompts.ToolResponse{
			{CallID: "call-1", Content: "one"},
			{CallID: "call-2", Content: "two"},
		}},
	}

	// Execute: convert Horizon prompts through the public message converter.
	messages, err := openai.ChatCompletionsMessages(context.Background(), input, nil)

	// Assert: each prompt is mapped once, except the tool prompt's two results.
	require.NoError(t, err)
	require.Len(t, messages, 6)
	require.Equal(t, "question", messages[0].OfUser.Content.OfArrayOfContentParts[0].OfText.Text)
	require.Equal(t, "answer", messages[1].OfAssistant.Content.OfString.Value)
	require.Equal(t, "call-1", messages[1].OfAssistant.ToolCalls[0].OfFunction.ID)
	require.Equal(t, "system", messages[2].OfSystem.Content.OfString.Value)
	require.Equal(t, "developer", messages[3].OfDeveloper.Content.OfString.Value)
	require.Equal(t, "call-1", messages[4].OfTool.ToolCallID)
	require.Equal(t, "one", messages[4].OfTool.Content.OfString.Value)
	require.Equal(t, "call-2", messages[5].OfTool.ToolCallID)
	require.Equal(t, "two", messages[5].OfTool.Content.OfString.Value)
}

// Checks that response choices map to Horizon messages, citations, refusals, and tool calls.
func TestChatResponseConversion(t *testing.T) {
	// Setup: serve choices that exercise both assistant content and tool-call mappings.
	client := newChatFixtureClient(t, `{
		"id":"chat-conversion",
		"object":"chat.completion",
		"created":1735689600,
		"model":"fixture-model",
		"service_tier":"default",
		"choices":[
			{
				"index":0,
				"finish_reason":"stop",
				"message":{
					"role":"assistant",
					"content":"Partial answer.",
					"refusal":"Cannot continue.",
					"annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":7,"title":"Source","url":"https://example.com"}}]
				}
			},
			{
				"index":1,
				"finish_reason":"tool_calls",
				"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}
			},
			{
				"index":2,
				"finish_reason":"stop",
				"message":{"role":"assistant","content":null,"refusal":"Cannot answer."}
			}
		],
		"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7,"cost":0.02}
	}`)

	// Execute: decode the SDK response using the production Generate method.
	response, err := client.Generate(context.Background(), &provider.Request{Model: "fixture-model"})

	// Assert: metadata, usage, citations, refusal content, and function calls survive mapping.
	require.NoError(t, err)
	require.Equal(t, "chat-conversion", response.ID)
	require.Equal(t, "fixture-model", response.Model)
	require.Equal(t, int64(4), response.Usage.InputTokens)
	require.Equal(t, int64(3), response.Usage.OutputTokens)
	require.Equal(t, int64(7), response.Usage.TotalTokens)
	require.InDelta(t, 0.02, response.Usage.APICost, 0.000001)
	require.Equal(t, "default", response.Meta["service_tier"])
	require.Len(t, response.Output, 3)
	require.Equal(t, prompts.TypeRefusal, response.Output[0].Type)
	require.Equal(t, "Partial answer.", response.Output[0].Content)
	require.Equal(t, "Cannot continue.", response.Output[0].Meta["refusal"])
	require.Equal(t, []prompts.Citation{{StartIndex: 0, EndIndex: 7, Title: "Source", URL: "https://example.com"}}, response.Output[0].Citations)
	require.Equal(t, prompts.TypeToolCall, response.Output[1].Type)
	require.Equal(t, "call-1", response.Output[1].ToolCalls[0].CallID)
	require.Equal(t, "lookup", response.Output[1].ToolCalls[0].Name)
	require.Equal(t, "{}", response.Output[1].ToolCalls[0].Arguments)
	require.Equal(t, prompts.TypeRefusal, response.Output[2].Type)
	require.Equal(t, "Cannot answer.", response.Output[2].Content)
}

// Checks the failure cases that are part of Chat Completions response conversion.
func TestChatResponseConversionErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{
			name: "no choices",
			body: `{"id":"empty","object":"chat.completion","created":1,"model":"fixture-model","choices":[]}`,
			want: errors.ErrNoChatCompletions,
		},
		{
			name: "unknown tool-call type",
			body: `{"id":"bad-tool","object":"chat.completion","created":1,"model":"fixture-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"computer","function":{"name":"run","arguments":"{}"}}]}}]}`,
			want: errors.ErrUnsupportedChatToolCallType,
		},
		{
			name: "invalid API cost",
			body: `{"id":"bad-cost","object":"chat.completion","created":1,"model":"fixture-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"cost":"invalid"}}`,
			want: strconv.ErrSyntax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: decode the fixture via the SDK and adapter.
			client := newChatFixtureClient(t, tt.body)
			_, err := client.Generate(context.Background(), &provider.Request{Model: "fixture-model"})

			// Assert: callers can identify the expected conversion failure.
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// Checks conversion of Horizon request options, tools, and structured output.
func TestChatRequestBodyConversion(t *testing.T) {
	// Setup: exercise the parameter branches and the function-tool request path.
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
			"frequency_penalty":      0.1,
			"logprobs":               true,
			"max_completion_tokens":  64,
			"max_tokens":             32,
			"presence_penalty":       0.2,
			"seed":                   7,
			"temperature":            0.3,
			"top_logprobs":           2,
			"top_p":                  0.9,
			"prompt_cache_key":       "cache-key",
			"safety_identifier":      "safety-id",
			"logit_bias":             map[string]any{"50256": -100},
			"prompt_cache_retention": "24h",
			"reasoning_effort":       "low",
			"service_tier":           "default",
			"verbosity":              "low",
		}),
	}

	// Execute: construct the SDK request body from the provider-neutral request.
	body, err := openai.ChatCompletionsBody(context.Background(), request)

	// Assert: options, tool behavior, and response schema are preserved.
	require.NoError(t, err)
	require.Equal(t, "fixture-model", body.Model)
	require.Equal(t, int64(1), body.N.Value)
	require.InDelta(t, 0.1, body.FrequencyPenalty.Value, 0.0001)
	require.True(t, body.Logprobs.Value)
	require.Equal(t, int64(64), body.MaxCompletionTokens.Value)
	require.Equal(t, int64(32), body.MaxTokens.Value)
	require.InDelta(t, 0.2, body.PresencePenalty.Value, 0.0001)
	require.Equal(t, int64(7), body.Seed.Value)
	require.InDelta(t, 0.3, body.Temperature.Value, 0.0001)
	require.Equal(t, int64(2), body.TopLogprobs.Value)
	require.InDelta(t, 0.9, body.TopP.Value, 0.0001)
	require.Equal(t, "cache-key", body.PromptCacheKey.Value)
	require.Equal(t, "safety-id", body.SafetyIdentifier.Value)
	require.Equal(t, int64(-100), body.LogitBias["50256"])
	require.Equal(t, "24h", string(body.PromptCacheRetention))
	require.Equal(t, "low", string(body.ReasoningEffort))
	require.Equal(t, "default", string(body.ServiceTier))
	require.Equal(t, "low", string(body.Verbosity))
	require.Equal(t, "auto", body.ToolChoice.OfAuto.Value)
	require.True(t, body.ParallelToolCalls.Value)
	require.Equal(t, "object", body.Tools[0].OfFunction.Function.Parameters["type"])
	require.True(t, body.Tools[0].OfFunction.Function.Strict.Value)
	require.Equal(t, "result", body.ResponseFormat.OfJSONSchema.JSONSchema.Name)
	schemaMap, ok := body.ResponseFormat.OfJSONSchema.JSONSchema.Schema.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", schemaMap["type"])
}

// Checks every supported output-schema response-format branch and its errors.
func TestChatSchemaFormatConversion(t *testing.T) {
	tests := []struct {
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
			name:  "invalid JSON schema",
			input: &schema.Schema{MimeType: mime.ApplicationSchemaJSON, Data: `null`},
			want:  errors.ErrInvalidJSONSchema,
		},
		{
			name:  "unsupported",
			input: &schema.Schema{MimeType: mime.TextHTML},
			want:  errors.ErrUnsupportedOutputSchemaMimeType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute: convert the schema to its Chat Completions response format.
			format, err := openai.ChatCompletionsResponseFormat(tt.input)

			// Assert: the selected SDK variant or wrapped schema error is returned.
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
				schemaMap, ok := format.OfJSONSchema.JSONSchema.Schema.(map[string]any)
				require.True(t, ok)
				require.Equal(t, "object", schemaMap["type"])
			}
		})
	}
}

// Checks tool schema conversion, including default schemas and contextual errors.
func TestChatToolDefinitionConversion(t *testing.T) {
	request := func(input *schema.Schema) *provider.Request {
		return &provider.Request{Tools: []capabilities.Definition{{Name: "lookup", InputSchema: input}}}
	}

	// Execute and assert: an omitted schema becomes an object with strict disabled.
	body, err := openai.ChatCompletionsBody(context.Background(), request(nil))
	require.NoError(t, err)
	require.Equal(t, "object", body.Tools[0].OfFunction.Function.Parameters["type"])
	require.False(t, body.Tools[0].OfFunction.Function.Strict.Value)

	// Execute and assert: malformed schemas preserve the tool name and root cause.
	_, err = openai.ChatCompletionsBody(context.Background(), request(&schema.Schema{Data: `null`}))
	require.ErrorContains(t, err, `tool "lookup" input schema`)
	require.ErrorIs(t, err, errors.ErrInvalidJSONSchema)
}

// Checks all Chat Completions finish-reason to Horizon type mappings.
func TestChatFinishReasonConversion(t *testing.T) {
	tests := []struct {
		reason string
		want   prompts.Type
	}{
		{reason: "stop", want: prompts.TypeMessage},
		{reason: "length", want: prompts.TypeMessage},
		{reason: "tool_calls", want: prompts.TypeToolCall},
		{reason: "function_call", want: prompts.TypeToolCall},
		{reason: "content_filter", want: prompts.TypeContentFilter},
		{reason: "unknown", want: prompts.TypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			// Execute and assert: the provider finish reason maps to its prompt type.
			require.Equal(t, tt.want, openai.FinishReasonType(tt.reason))
		})
	}
}

// Checks malformed message roles and empty tool results fail request conversion.
func TestChatPromptConversionErrors(t *testing.T) {
	_, err := openai.ChatCompletionsMessages(context.Background(), prompts.Prompts{{Role: prompts.RoleTool}}, nil)
	require.ErrorIs(t, err, errors.ErrToolMessageMissingResults)

	_, err = openai.ChatCompletionsMessages(context.Background(), prompts.Prompts{{Role: prompts.RoleUnknown}}, nil)
	require.ErrorIs(t, err, errors.ErrUnsupportedMessageRole)
}

// newChatFixtureClient serves a deterministic Chat Completions response through a test server.
func newChatFixtureClient(t *testing.T, body string) *openai.ChatCompletionsClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := openai.NewChatCompletions(provider.Config{
		InferenceEndpoint: server.URL + "/",
		Credentials:       auth.NewAPIKey("fixture-key"),
	}, provider.ResolveOptions())
	require.NoError(t, err)
	return client
}

// generateWithOpenRouterModels retries generation using the supplied model list
// when a failure is classified as retryable by the test environment.
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
