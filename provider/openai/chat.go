package openai

import (
	"context"
	"encoding/base64"

	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"

	"go.rtnl.ai/x/mime"
)

// Client for the OpenAI Chat Completions API.
type ChatCompletionsClient struct {
	client *openai.Client
}

// NewChatCompletions creates a client from the configuration and resolved provider options.
func NewChatCompletions(conf provider.Config, options provider.Options) (*ChatCompletionsClient, error) {
	client, err := New(conf, options)
	if err != nil {
		return nil, err
	}
	return &ChatCompletionsClient{client: client}, nil
}

// Generates a response from the OpenAI Chat Completions API.
func (cc *ChatCompletionsClient) Generate(ctx context.Context, req *provider.Request) (out *provider.Response, err error) {
	// Create the request body to send via the OpenAI chat completions API.
	var body openai.ChatCompletionNewParams
	if body, err = ChatCompletionsBody(ctx, req); err != nil {
		return nil, err
	}

	// Execute the chat completions API request.
	conf, err := config.Get()
	if err != nil {
		return nil, err
	}
	var completion *openai.ChatCompletion
	if completion, err = cc.client.Chat.Completions.New(ctx, body, option.WithRequestTimeout(conf.ProviderRequestTimeout)); err != nil {
		// TODO: handle errors in a standardized way
		return nil, err
	}

	if len(completion.Choices) == 0 {
		return nil, errors.ErrNoChatCompletions
	}

	// TODO: Better handling of the Usage data, just collecting simple info right now.
	out = &provider.Response{
		ID:    completion.ID,
		Model: completion.Model,
		Usage: provider.Usage{
			InputTokens:  completion.Usage.PromptTokens,
			OutputTokens: completion.Usage.CompletionTokens,
			TotalTokens:  completion.Usage.TotalTokens,
		},
		Created: time.Unix(completion.Created, 0),
		Meta:    make(provider.Meta),
	}
	out.Meta.PutString("object", completion.Object)
	out.Meta.PutString("service_tier", completion.ServiceTier)

	// Include API cost if provided
	// TODO: The OpenAI Go SDK isn't properly reporting fields as valid, so we have to
	// do the valid checking manually.
	if cost, ok := completion.Usage.JSON.ExtraFields["cost"]; ok && cost.Raw() != "" {
		if out.Usage.APICost, err = strconv.ParseFloat(cost.Raw(), 64); err != nil {
			return nil, err
		}
	}

	for _, choice := range completion.Choices {
		// TODO: Omit index and logprobs metadata when they contain no useful
		// provider data.
		meta := make(provider.Meta)
		meta.PutString("finish_reason", choice.FinishReason)
		meta["index"] = choice.Index
		meta["logprobs"] = choice.Logprobs

		msg := &prompts.Prompt{
			Role: prompts.RoleAssistant, // Always assistant role for chat completions.
			Type: FinishReasonType(choice.FinishReason),
			Meta: meta,
		}

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
			if msg.Type == prompts.TypeUnknown || msg.Type == prompts.TypeMessage {
				msg.Type = prompts.TypeRefusal
			}
		}

		if len(choice.Message.Annotations) > 0 {
			msg.Citations = make([]prompts.Citation, 0, len(choice.Message.Annotations))
			for _, annotation := range choice.Message.Annotations {
				msg.Citations = append(msg.Citations, prompts.Citation{
					StartIndex: annotation.URLCitation.StartIndex,
					EndIndex:   annotation.URLCitation.EndIndex,
					Title:      annotation.URLCitation.Title,
					URL:        annotation.URLCitation.URL,
				})
			}
		}

		// TODO: map generated audio into response attachments

		for _, call := range choice.Message.ToolCalls {
			switch call.Type {
			case "function":
				msg.ToolCalls = append(msg.ToolCalls, prompts.ToolCall{
					CallID:    call.ID,
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				})
			default:
				return nil, fmt.Errorf("%w: %s", errors.ErrUnsupportedChatToolCallType, call.Type)
			}
		}

		out.Output = append(out.Output, msg)
	}

	return out, nil
}

// Create a openai.ChatCompletionNewParams body from a horizon.Request.
//
// Handling notes follow.
//
// - Store: always false for Endeavor
// - ParallelToolCalls: enabled when tools are advertised; Horizon performs calls directly
// - User: deprecated, and so unused in Endeavor
// - Metadata: currently unused in Endeavor
// - Modalities: currently unused in Endeavor (may be implemented in the future)
// - StreamOptions: currently unused in Endeavor (Endeavor does not use streaming)
// - FunctionCall: deprecated, and so unused in Endeavor
// - Functions: deprecated, and so unused in Endeavor
// - Prediction: currently unused in Endeavor (may be implemented in the future)
//
// - MaxCompletionTokens: will lookup first max_completion_tokens and then max_tokens.
// - MaxTokens: only set if specified directly because it is deprecated.
// - N: always set to 1 because in an Endeavor context, we only ever want a single generation.
//
// NOTE: we have to keep this function up to date with any OpenAI API changes.
// Creates a Chat Completions request body, using ctx when resolving remote
// attachments.
func ChatCompletionsBody(ctx context.Context, req *provider.Request) (body openai.ChatCompletionNewParams, err error) {
	// TODO: handle Audio once represented on request
	// TODO: handle WebSearchOptions once represented on request
	body = openai.ChatCompletionNewParams{
		Store: openai.Bool(false),
		Model: req.Model,
		N:     openai.Int(1), // In an Endeavor context, we only ever want a single generation.
	}

	if body.Messages, err = ChatCompletionsMessages(ctx, req.Input, req.Attachments); err != nil {
		return body, err
	}

	if len(req.Tools) > 0 {
		if body.Tools, err = chatTools(req.Tools); err != nil {
			return body, err
		}
		// TODO: give an option to "require" tool use; "auto" means the model chooses to use tools or not
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
func ChatCompletionsMessages(ctx context.Context, in prompts.Prompts, attachments attachments.Attachments) (out []openai.ChatCompletionMessageParamUnion, err error) {
	var attachParts []openai.ChatCompletionContentPartUnionParam
	if attachParts, err = ChatCompletionsAttachments(ctx, attachments); err != nil {
		return nil, err
	}

	in, attachmentIndex := attachmentMessages(in, len(attachments) > 0)
	out = make([]openai.ChatCompletionMessageParamUnion, 0, len(in))
	for i, msg := range in {
		if msg == nil {
			continue
		}
		param := openai.ChatCompletionMessageParamUnion{}
		switch msg.Role {
		case prompts.RoleUser:
			userParts := []openai.ChatCompletionContentPartUnionParam{
				{
					OfText: &openai.ChatCompletionContentPartTextParam{
						Text: msg.Content,
					},
				},
			}
			if i == attachmentIndex {
				userParts = append(userParts, attachParts...)
			}
			param.OfUser = &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: userParts,
				},
			}
		case prompts.RoleAssistant:
			param.OfAssistant = &openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
				ToolCalls: chatToolCalls(msg.ToolCalls),
			}
		case prompts.RoleSystem:
			param.OfSystem = &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
		case prompts.RoleTool:
			if len(msg.ToolResults) == 0 {
				return nil, errors.ErrToolMessageMissingResults
			}
			for _, result := range msg.ToolResults {
				out = append(out, openai.ChatCompletionMessageParamUnion{
					OfTool: &openai.ChatCompletionToolMessageParam{
						Content:    openai.ChatCompletionToolMessageParamContentUnion{OfString: openai.String(result.Content)},
						ToolCallID: result.CallID,
					},
				})
			}
			// There can be multiple tool results, so we need to append them all
			// and then skip the final append with this 'continue'.
			continue
		case prompts.RoleDeveloper:
			param.OfDeveloper = &openai.ChatCompletionDeveloperMessageParam{
				Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
		default:
			return nil, fmt.Errorf("%w: %s", errors.ErrUnsupportedMessageRole, msg.Role)
		}

		out = append(out, param)
	}
	return out, nil
}

// chatTools maps provider-neutral tool definitions to Chat Completions API tools.
func chatTools(definitions []capabilities.Definition) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(definitions) == 0 {
		return nil, nil
	}

	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		parameters, err := definition.InputSchema.Object()
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", definition.Name, err)
		}
		function := shared.FunctionDefinitionParam{
			Name:        string(definition.Name),
			Description: openai.String(definition.Description),
			Parameters:  shared.FunctionParameters(parameters),
			Strict:      openai.Bool(definition.InputSchema != nil && definition.InputSchema.Strict),
		}
		tools = append(tools, openai.ChatCompletionFunctionTool(function))
	}
	return tools, nil
}

// chatToolCalls maps provider-neutral tool calls to Chat Completions API function calls.
func chatToolCalls(calls []prompts.ToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	out := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, call := range calls {
		out = append(out, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.CallID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Name,
					Arguments: call.Arguments,
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
	if schema == nil || schema.MimeType.IsUnknown() || schema.MimeType == mime.TextPlain {
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	}

	// Switch on the mime type of the schema
	switch schema.MimeType {
	case mime.ApplicationJSON:
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}, nil
	case mime.ApplicationSchemaJSON:
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

		// Resolve the JSON schema, fetching from a remote URI when supported.
		if format.OfJSONSchema.JSONSchema.Schema, err = schema.Object(); err != nil {
			return format, err
		}
		return format, nil
	default:
		return format, fmt.Errorf("%w: %s", errors.ErrUnsupportedOutputSchemaMimeType, schema.MimeType)
	}
}

// Converts the OpenAI finish reason string to a Horizon output type.
func FinishReasonType(reason string) prompts.Type {
	switch reason {
	case "stop", "length":
		return prompts.TypeMessage
	case "tool_calls", "function_call":
		return prompts.TypeToolCall
	case "content_filter":
		return prompts.TypeContentFilter
	default:
		return prompts.TypeUnknown
	}
}

// Converts Horizon attachments to Chat Completions content parts, using ctx for
// remote attachment downloads.
func ChatCompletionsAttachments(ctx context.Context, attachments attachments.Attachments) (parts []openai.ChatCompletionContentPartUnionParam, err error) {
	var uri string
	for _, attachment := range attachments {
		if attachment == nil {
			return nil, errors.ErrNilAttachment
		}
		contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(attachment.ContentType, ";", 2)[0]))
		topLevelType, _, _ := strings.Cut(contentType, "/")

		switch topLevelType {
		case "text":
			var text string
			if text, err = attachment.Text(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfText: &openai.ChatCompletionContentPartTextParam{
					Text: text,
				},
			})
		case "image":
			if uri, err = attachment.Base64URI(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
						URL: uri,
					},
				},
			})
		case "audio":
			// TODO: Needs further testing since there are some inconsistencies with
			// how audio files are handled across different model providers.
			var format string
			switch contentType {
			case "audio/mpeg", "audio/mp3":
				format = "mp3"
			case "audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave":
				format = "wav"
			default:
				return nil, fmt.Errorf("%w: %s", errors.ErrUnsupportedChatCompletionsAudioType, attachment.ContentType)
			}

			var data []byte
			if data, err = attachment.Bytes(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfInputAudio: &openai.ChatCompletionContentPartInputAudioParam{
					InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
						Data:   base64.StdEncoding.EncodeToString(data),
						Format: format,
					},
				},
			})
		default:
			if uri, err = attachment.Base64URI(ctx); err != nil {
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
