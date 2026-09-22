package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/media"
	"go.rtnl.ai/horizon/prompts"
	api "go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/schema"
)

var (
	ErrNoChatCompletions = errors.New("no chat completions choices returned")
)

// Client for the OpenAI Chat Completions API.
type ChatCompletionsClient struct {
	client *openai.Client
}

// Creates a new [ChatCompletionsClient] from the given client configuration.
func NewChatCompletions(conf config.Provider) (cc *ChatCompletionsClient, err error) {
	cc = &ChatCompletionsClient{}
	if cc.client, err = New(conf); err != nil {
		return nil, err
	}
	return cc, nil
}

// Generate generates a response from the OpenAI chat completions API.
func (cc *ChatCompletionsClient) Generate(ctx context.Context, req *api.Request) (out *api.Response, err error) {
	// Create the request body to send via the OpenAI chat completions API.
	var body openai.ChatCompletionNewParams
	if body, err = ChatCompletionsBody(req); err != nil {
		return nil, err
	}

	// Execute the chat completions API request.
	var completion *openai.ChatCompletion
	if completion, err = cc.client.Chat.Completions.New(ctx, body, option.WithRequestTimeout(config.DefaultTimeout)); err != nil {
		// TODO: handle errors in a standardized way
		return nil, err
	}

	if len(completion.Choices) == 0 {
		return nil, ErrNoChatCompletions
	}

	// TODO: Better handling of the Usage data, just collecting simple info right now.
	out = &api.Response{
		ID:    completion.ID,
		Model: completion.Model,
		Usage: api.Usage{
			InputTokens:  completion.Usage.PromptTokens,
			OutputTokens: completion.Usage.CompletionTokens,
			TotalTokens:  completion.Usage.TotalTokens,
		},
		Created: time.Unix(completion.Created, 0),
		Meta:    make(map[string]any),
	}

	// Include API cost if provided
	// TODO: The OpenAI Go SDK isn't properly reporting fields as valid, so we have to
	// do the valid checking manually.
	if cost, ok := completion.Usage.JSON.ExtraFields["cost"]; ok && cost.Raw() != "" {
		if out.Usage.APICost, err = strconv.ParseFloat(cost.Raw(), 64); err != nil {
			return nil, err
		}
	}

	// Update the metdata data of the response.
	out.Meta.PutString("object", completion.Object)
	out.Meta.PutString("service_tier", completion.ServiceTier)

	for _, choice := range completion.Choices {
		msg := api.Output{
			Role: api.RoleAssistant, // Always assistant role for chat completions.
			Type: FinishReasonType(choice.FinishReason),
			Meta: make(api.Meta),
		}

		// TODO: Don't add empty index or logprobs to metadata.
		msg.Meta.PutString("finish_reason", choice.FinishReason)
		msg.Meta["index"] = choice.Index
		msg.Meta["logprobs"] = choice.Logprobs

		// The message can contain content or refusal (and possibly both). If there is
		// no content but a refusal, the refusal is the content.
		msg.Content = choice.Message.Content
		if choice.Message.Refusal != "" {
			// If there is a refusal set it in the output message
			if msg.Content == "" {
				msg.Content = choice.Message.Refusal
			} else {
				msg.Meta["refusal"] = choice.Message.Refusal
			}

			// If there is a refusal, set the type to refusal if set to message or unknown.
			if msg.Type == api.OutputUnknown || msg.Type == api.OutputMessage {
				msg.Type = api.OutputRefusal
			}
		}

		// Handle annotations on the message (which in chat completions is always URL citations)
		if len(choice.Message.Annotations) > 0 {
			msg.Citations = make([]api.Citation, 0, len(choice.Message.Annotations))
			for _, annotation := range choice.Message.Annotations {
				msg.Citations = append(msg.Citations, api.Citation{
					StartIndex: annotation.URLCitation.StartIndex,
					EndIndex:   annotation.URLCitation.EndIndex,
					Title:      annotation.URLCitation.Title,
					URL:        annotation.URLCitation.URL,
				})
			}
		}

		// Convert chat completion tool calls to Horizon tool calls.
		// TODO: Handle Audio if set
		var toolCalls []capabilities.ToolCall
		for _, call := range choice.Message.ToolCalls {
			switch call.Type {
			case "function":
				toolCalls = append(toolCalls, capabilities.ToolCall{
					CallID:    call.ID,
					Name:      call.Function.Name,
					Arguments: json.RawMessage(call.Function.Arguments),
				})
			default:
				return nil, fmt.Errorf("unsupported chat completion tool call type: %s", call.Type)
			}
		}
		out.ToolCalls = append(out.ToolCalls, toolCalls...)

		out.Output = append(out.Output, msg)
	}

	return out, nil
}

// Create a openai.ChatCompletionNewParams body from a api.Request.
//
// Handling notes follow.
//
// - Store: always false for Horizon
// - ParallelToolCalls: enabled when tools are advertised; Horizon performs calls directly
// - User: deprecated, and so unused in Horizon
// - Metadata: currently unused in Horizon
// - Modalities: currently unused in Horizon (may be implemented in the future)
// - StreamOptions: currently unused in Horizon (Horizon does not use streaming)
// - FunctionCall: deprecated, and so unused in Horizon
// - Functions: deprecated, and so unused in Horizon
// - Prediction: currently unused in Horizon (may be implemented in the future)
//
// - MaxCompletionTokens: will lookup first max_completion_tokens and then max_tokens.
// - MaxTokens: only set if specified directly because it is deprecated.
// - N: always set to 1 because in an Horizon context, we only ever want a single generation.
//
// NOTE: we have to keep this function up to date with any OpenAI API changes.
func ChatCompletionsBody(req *api.Request) (body openai.ChatCompletionNewParams, err error) {
	// TODO: handle Audio
	// TODO: handle WebSearchOptions
	body = openai.ChatCompletionNewParams{
		Store: openai.Bool(false),
		Model: req.Model,
		N:     openai.Int(1), // In an Horizon context, we only ever want a single generation.
	}

	if body.Messages, err = ChatCompletionsMessages(req.Input, req.Attachments); err != nil {
		return body, err
	}

	if body.Tools, err = ChatCompletionsTools(req.Tools); err != nil {
		return body, err
	}
	if len(req.Tools) > 0 {
		body.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("auto"),
		}
		body.ParallelToolCalls = openai.Bool(true)
	}

	if body.ResponseFormat, err = ChatCompletionsResponseFormat(req.OutputSchema); err != nil {
		return body, err
	}

	// If there are no parameters on the request, return the body as is.
	if req.Params == nil || req.Params.Len() == 0 {
		return body, nil
	}

	// Handle all of the parameters on the request.
	if frequencyPenalty, ok := req.Params.FrequencyPenalty(); ok {
		body.FrequencyPenalty = openai.Float(frequencyPenalty)
	}

	if logProbs, ok := req.Params.LogProbs(); ok {
		body.Logprobs = openai.Bool(logProbs)
	}

	// This will lookup first max_completion_tokens and then max_tokens.
	// NOTE: max_tokens is deprecated and not used in the chat_completions API.
	if maxTokens, ok := req.Params.MaxCompletionTokens(); ok {
		body.MaxCompletionTokens = openai.Int(maxTokens)
	}

	// If max_tokens is specifically set, then add it to the body.
	// See above: max_completion_tokens should be used instead.
	if maxTokens, ok := req.Params.MaxTokens(); ok {
		body.MaxTokens = openai.Int(maxTokens)
	}

	if presencePenalty, ok := req.Params.PresencePenalty(); ok {
		body.PresencePenalty = openai.Float(presencePenalty)
	}

	if seed, ok := req.Params.Seed(); ok {
		body.Seed = openai.Int(seed)
	}

	if temperature, ok := req.Params.Temperature(); ok {
		body.Temperature = openai.Float(temperature)
	}

	if topLogProbs, ok := req.Params.TopLogProbs(); ok {
		body.TopLogprobs = openai.Int(topLogProbs)
	}

	if topP, ok := req.Params.TopP(); ok {
		body.TopP = openai.Float(topP)
	}

	if promptCacheKey, ok := req.Params.PromptCacheKey(); ok {
		body.PromptCacheKey = openai.String(promptCacheKey)
	}

	if safetyIdentifier, ok := req.Params.SafetyIdentifier(); ok {
		body.SafetyIdentifier = openai.String(safetyIdentifier)
	}

	if logitBias, ok := req.Params.LogitBias(); ok {
		body.LogitBias = logitBias
	}

	if promptCacheRetention, ok := req.Params.PromptCacheRetention(); ok {
		body.PromptCacheRetention = openai.ChatCompletionNewParamsPromptCacheRetention(promptCacheRetention)
	}

	if reasoningEffort, ok := req.Params.ReasoningEffort(); ok {
		body.ReasoningEffort = shared.ReasoningEffort(reasoningEffort)
	}

	if serviceTier, ok := req.Params.ServiceTier(); ok {
		body.ServiceTier = openai.ChatCompletionNewParamsServiceTier(serviceTier)
	}

	if verbosity, ok := req.Params.Verbosity(); ok {
		body.Verbosity = openai.ChatCompletionNewParamsVerbosity(verbosity)
	}

	return body, nil
}

// Creates a list of OpenAI chat completion message from a list of Horizon
// messages.
//
// NOTE: the `Name` field is not supported by Horizon.
// TODO: handle the attachments in the arrays of the message params
func ChatCompletionsMessages(in prompts.Prompts, attachments []*api.Attachment) (out []openai.ChatCompletionMessageParamUnion, err error) {
	// Handle the attachment parts
	var parts []openai.ChatCompletionContentPartUnionParam
	if parts, err = ChatCompletionsAttachments(attachments); err != nil {
		return nil, err
	}

	out = make([]openai.ChatCompletionMessageParamUnion, 0, len(in))
	for _, msg := range in {
		param := openai.ChatCompletionMessageParamUnion{}
		switch msg.Role {
		case api.RoleUser:
			userParts := append([]openai.ChatCompletionContentPartUnionParam{
				{
					OfText: &openai.ChatCompletionContentPartTextParam{
						Text: msg.Content,
					},
				},
			}, parts...)
			param.OfUser = &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: userParts,
				},
			}
		case api.RoleAssistant:
			param.OfAssistant = &openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
				ToolCalls: chatToolCalls(msg.ToolCalls),
			}
		case api.RoleSystem:
			param.OfSystem = &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
		case api.RoleTool:
			if len(msg.ToolResults) == 0 {
				return nil, errors.New("tool message must contain tool results")
			}
			for _, result := range msg.ToolResults {
				out = append(out, openai.ChatCompletionMessageParamUnion{
					OfTool: &openai.ChatCompletionToolMessageParam{
						Content:    openai.ChatCompletionToolMessageParamContentUnion{OfString: openai.String(toolResultText(result))},
						ToolCallID: result.CallID,
					},
				})
			}
			continue
		case api.RoleDeveloper:
			param.OfDeveloper = &openai.ChatCompletionDeveloperMessageParam{
				Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
		default:
			return nil, fmt.Errorf("unsupported message role: %v", msg.Role)
		}

		out = append(out, param)
	}
	return out, nil
}

// ChatCompletionsTools maps provider-neutral tool definitions to function tools
// accepted by the Chat Completions API.
func ChatCompletionsTools(definitions []capabilities.ToolDefinition) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(definitions) == 0 {
		return nil, nil
	}
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		parameters, err := toolParameters(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", definition.Name, err)
		}
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        definition.Name,
			Description: openai.String(definition.Description),
			Parameters:  shared.FunctionParameters(parameters),
		}))
	}
	return tools, nil
}

// chatToolCalls maps Horizon tool calls to Chat Completions function calls.
func chatToolCalls(calls []capabilities.ToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	out := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, call := range calls {
		out = append(out, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.CallID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Name,
					Arguments: string(call.Arguments),
				},
			},
		})
	}
	return out
}

// Creates an OpenAI chat completion response format from a Horizon schema. The
// response format can be a nil schema, text/plain, application/json, or
// application/schema+json. A schema is only added to the request if the mime
// type is application/schema+json.
func ChatCompletionsResponseFormat(schema *schema.Schema) (format openai.ChatCompletionNewParamsResponseFormatUnion, err error) {
	// If the schema is nil, or plain text is requested, return text output format by default.
	if schema == nil {
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	}
	mimeType := media.Type(schema.MimeType.String())
	if mimeType.IsUnknown() || mimeType == media.TextPlain {
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	}

	// Switch on the mime type of the schema
	switch mimeType {
	case media.ApplicationJSON:
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}, nil
	case media.ApplicationSchemaJSON:
		format = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					// The name of the response format. Must be a-z, A-Z, 0-9, or contain underscores
					// and dashes, with a maximum length of 64.
					Name: schema.Name,

					// Whether to enable strict schema adherence when generating the output. If set to
					// true, the model will always follow the exact schema defined in the `schema`
					// field. Only a subset of JSON Schema is supported when `strict` is `true`. To
					// learn more, read the
					// [Structured Outputs guide](https://platform.openai.com/docs/guides/structured-outputs).
					Strict: openai.Bool(schema.Strict),

					// A description of what the response format is for, used by the model to determine
					// how to respond in the format.
					Description: openai.String(schema.Description),
				},
			},
		}

		// Resolve and validate the JSON schema, fetching from a remote URI if necessary.
		if format.OfJSONSchema.JSONSchema.Schema, err = schema.JSON(); err != nil {
			return format, err
		}
		return format, nil
	default:
		return format, fmt.Errorf("unsupported output schema mime type: %s", mimeType)
	}
}

// Converts the OpenAI finish reason string to a Horizon output type.
func FinishReasonType(reason string) api.OutputType {
	switch reason {
	case "stop", "length":
		return api.OutputMessage
	case "tool_calls", "function_call":
		return api.OutputToolCall
	case "content_filter":
		return api.OutputContentFilter
	default:
		return api.OutputUnknown
	}
}

// Convert Horizon attachments to OpenAI content parts
func ChatCompletionsAttachments(attachments []*api.Attachment) (parts []openai.ChatCompletionContentPartUnionParam, err error) {
	var uri string
	for _, attachment := range attachments {
		switch attachment.MimeType.Top() {
		case "text":
			var text string
			if text, err = attachment.Text(); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfText: &openai.ChatCompletionContentPartTextParam{
					Text: text,
				},
			})
		case "image":
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
						URL: attachment.URL,
					},
				},
			})
		case "audio":
			// TODO: Needs further testing since there are some inconsistencies with
			// how audio files are handled across different model providers.
			if uri, err = attachment.Base64URI(); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfInputAudio: &openai.ChatCompletionContentPartInputAudioParam{
					InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
						Data:   uri,
						Format: attachment.MimeType.String(),
					},
				},
			})
		default:
			if uri, err = attachment.Base64URI(); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfFile: &openai.ChatCompletionContentPartFileParam{
					File: openai.ChatCompletionContentPartFileFileParam{
						FileData: openai.String(uri),
						Filename: openai.String(attachment.Filename),
					},
				},
			})
		}
	}
	return parts, nil
}
