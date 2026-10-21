package openai

import (
	"context"
	"encoding/json"
	"log/slog"

	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/metadata"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"

	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/rlog"
)

// Responses reasoning items must be replayed with their encrypted content when
// the adapter manages continuation through the prompt transcript, so we will
// save their exact content in the prompt metadata so we can replay them later.
const responsesReasoningItemMetaKey = "responses_reasoning_item"

// Client for the OpenAI Responses API.
type ResponsesClient struct {
	client *openai.Client
}

// NewResponses creates a client from the configuration and resolved provider options.
func NewResponses(conf provider.Config, options provider.Options) (*ResponsesClient, error) {
	client, err := New(conf, options)
	if err != nil {
		return nil, err
	}
	return &ResponsesClient{client: client}, nil
}

// Generates a response from the OpenAI Responses API.
func (rc *ResponsesClient) Generate(ctx context.Context, req *provider.Request) (out *provider.Response, err error) {
	// Create the request body to send via the OpenAI responses API.
	var body responses.ResponseNewParams
	if body, err = ResponsesBody(ctx, req); err != nil {
		return nil, err
	}

	// Execute the responses API request.
	conf, err := config.Get()
	if err != nil {
		return nil, err
	}
	var resp *responses.Response
	if resp, err = rc.client.Responses.New(ctx, body, option.WithRequestTimeout(conf.ProviderRequestTimeout)); err != nil {
		// TODO: handle errors in a standardized way
		return nil, err
	}

	if resp.Error.Message != "" {
		return nil, fmt.Errorf("%w: %s: %s", errors.ErrOpenAIResponsesError, resp.Error.Code, resp.Error.Message)
	}

	if len(resp.Output) == 0 {
		return nil, errors.ErrNoResponseOutput
	}

	// TODO: Better handling of the Usage data, just collecting simple info right now.
	out = &provider.Response{
		ID:    resp.ID,
		Model: string(resp.Model),
		Usage: provider.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
		Created: time.Unix(int64(resp.CreatedAt), 0),
		Meta:    make(provider.Meta),
	}
	out.Meta.PutString("object", resp.Object)
	out.Meta.PutString("service_tier", resp.ServiceTier)
	out.Meta.PutString("status", resp.Status)
	out.Meta.PutString("incomplete_reason", resp.IncompleteDetails.Reason)

	// Include API cost if provided
	// TODO: The OpenAI Go SDK isn't properly reporting fields as valid, so we have to
	// do the valid checking manually.
	if cost, ok := resp.Usage.JSON.ExtraFields["cost"]; ok && cost.Raw() != "" {
		if out.Usage.APICost, err = strconv.ParseFloat(cost.Raw(), 64); err != nil {
			return nil, err
		}
	}

	for _, item := range resp.Output {
		switch variant := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			meta := make(provider.Meta)
			meta.PutString("status", variant.Status)
			meta.PutString("phase", variant.Phase)

			msg := &prompts.Prompt{
				ID:     variant.ID,
				Role:   prompts.RoleAssistant, // Always assistant role for response messages.
				Status: string(variant.Status),
				Phase:  string(variant.Phase),
				Type:   ResponseStatusType(resp.Status, resp.IncompleteDetails.Reason),
				Meta:   metadata.Metadata(meta),
			}

			var logprobs []responses.ResponseOutputTextLogprob
			var annotations []responses.ResponseOutputTextAnnotationUnion
			for _, part := range variant.Content {
				switch part.Type {
				case "output_text":
					msg.Content += part.Text
					logprobs = append(logprobs, part.Logprobs...)
					annotations = append(annotations, part.Annotations...)
				case "refusal":
					// The message can contain content or refusal (and possibly both). If there is
					// no content but a refusal, the refusal is the content.
					if msg.Content == "" {
						msg.Content = part.Refusal
					} else {
						msg.Meta["refusal"] = part.Refusal
					}

					// If there is a refusal, set the type to refusal if set to message or unknown.
					if msg.Type == prompts.TypeUnknown || msg.Type == prompts.TypeMessage {
						msg.Type = prompts.TypeRefusal
					}
				}
			}

			if len(logprobs) > 0 {
				msg.Meta["logprobs"] = logprobs
			}

			// Handle annotations on the message. Responses can include several
			// annotation types; Horizon currently only maps URL citations.
			for _, annotation := range annotations {
				if annotation.Type != "url_citation" {
					continue
				}
				msg.Citations = append(msg.Citations, prompts.Citation{
					StartIndex: annotation.StartIndex,
					EndIndex:   annotation.EndIndex,
					Title:      annotation.Title,
					URL:        annotation.URL,
				})
			}

			out.Output = append(out.Output, msg)
		case responses.ResponseOutputItemImageGenerationCall:
			var attachment *attachments.Attachment
			if attachment, err = attachments.ParseBase64URI(variant.Result); err != nil {
				return nil, err
			}
			attachment.Filename = "image"
			out.Attachments = append(out.Attachments, attachment)
		case responses.ResponseFunctionToolCall:
			out.Output = append(out.Output, &prompts.Prompt{
				ID:     variant.ID,
				Role:   prompts.RoleAssistant,
				Status: string(variant.Status),
				Type:   prompts.TypeToolCall,
				ToolCalls: []prompts.ToolCall{{
					CallID:    variant.CallID,
					Name:      variant.Name,
					Arguments: variant.Arguments,
				}},
			})
		case responses.ResponseReasoningItem:
			meta := make(provider.Meta)
			meta[responsesReasoningItemMetaKey] = variant.RawJSON()
			var summary strings.Builder
			for _, item := range variant.Summary {
				if summary.Len() > 0 {
					summary.WriteByte('\n')
				}
				summary.WriteString(item.Text)
			}
			out.Output = append(out.Output, &prompts.Prompt{
				ID:      variant.ID,
				Role:    prompts.RoleAssistant,
				Type:    prompts.TypeReasoning,
				Content: summary.String(),
				Status:  string(variant.Status),
				Meta:    metadata.Metadata(meta),
			})
		default:
			// Other Responses output item types are not currently represented by prompts.
			rlog.DebugAttrs(ctx, "unhandled response item type", slog.Any("variant", variant))
		}
	}

	// Fall back to aggregated text if the response had output items but none were
	// mapped as messages (for example, providers that only populate output_text).
	if len(out.Output) == 0 {
		if text := resp.OutputText(); text != "" {
			out.Output = append(out.Output, &prompts.Prompt{
				Role:    prompts.RoleAssistant,
				Type:    ResponseStatusType(resp.Status, resp.IncompleteDetails.Reason),
				Content: text,
				Meta:    metadata.Metadata{},
			})
		} else {
			return nil, errors.ErrNoResponseOutput
		}
	}

	return out, nil
}

// Create a responses.ResponseNewParams body from a horizon.Request.
//
// Handling notes follow.
//
// - Background: always false for Endeavor
// - Store: always false for Endeavor
// - Instructions: currently unused in Endeavor
// - ParallelToolCalls: enabled when tools are advertised; Horizon performs calls directly
// - PreviousResponseID: currently unused in Endeavor
// - User: deprecated, and so unused in Endeavor
// - ContextManagement: currently unused in Endeavor
// - Conversation: currently unused in Endeavor
// - Metadata: currently unused in Endeavor
// - Prompt: currently unused in Endeavor (Endeavor performs prompt rendering directly)
// - StreamOptions: currently unused in Endeavor (Endeavor does not use streaming)
//
// - MaxOutputTokens: This will lookup first max_output_tokens and then max_tokens.
// - Reasoning: Endeavor uses ReasoningEffort and ReasoningSummary parameters.
// - Verbosity: Is controlled by the ResponseTextConfigParam.
//
// NOTE: we have to keep this function up to date with any OpenAI API changes.
// Creates a Responses API request body, using ctx when resolving remote
// attachments.
func ResponsesBody(ctx context.Context, req *provider.Request) (body responses.ResponseNewParams, err error) {
	body = responses.ResponseNewParams{
		Background: openai.Bool(false),
		Store:      openai.Bool(false),
		Model:      req.Model,
	}

	if len(req.Tools) > 0 {
		if body.Tools, err = ResponsesTools(req.Tools); err != nil {
			return body, err
		}
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
		body.ParallelToolCalls = openai.Bool(true)
	}

	if body.Input, err = ResponsesMessages(ctx, req.Input, req.Attachments); err != nil {
		return body, err
	}

	if body.Text.Format, err = ResponsesResponseFormat(req.OutputSchema); err != nil {
		return body, err
	}

	// If there are no parameters on the request, return the body as is.
	if req.Params == nil || req.Params.Len() == 0 {
		return body, nil
	}

	// This will lookup first max_output_tokens and then max_tokens.
	if maxOutputTokens, ok := req.Params.MaxOutputTokens(); ok {
		body.MaxOutputTokens = openai.Int(maxOutputTokens)
	}

	if temperature, ok := req.Params.Temperature(); ok {
		body.Temperature = openai.Float(temperature)
	}

	if topLogProbs, ok := req.Params.TopLogProbs(); ok {
		body.TopLogprobs = openai.Int(topLogProbs)
		body.Include = append(body.Include, responses.ResponseIncludableMessageOutputTextLogprobs)
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

	if promptCacheRetention, ok := req.Params.PromptCacheRetention(); ok {
		body.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention(promptCacheRetention)
	}

	if serviceTier, ok := req.Params.ServiceTier(); ok {
		body.ServiceTier = responses.ResponseNewParamsServiceTier(serviceTier)
	}

	if truncation, ok := req.Params.Truncation(); ok {
		body.Truncation = responses.ResponseNewParamsTruncation(truncation)
	}

	// Reasoning is only added if either ReasoningEffort or ReasoningSummary is set.
	var reasoning *shared.ReasoningParam
	if reasoningEffort, ok := req.Params.ReasoningEffort(); ok {
		reasoning = &shared.ReasoningParam{}
		reasoning.Effort = shared.ReasoningEffort(reasoningEffort)
	}

	if reasoningSummary, ok := req.Params.ReasoningSummary(); ok {
		if reasoning == nil {
			reasoning = &shared.ReasoningParam{}
		}
		reasoning.Summary = shared.ReasoningSummary(reasoningSummary)
	}

	if reasoning != nil {
		body.Reasoning = *reasoning
	}

	if verbosity, ok := req.Params.Verbosity(); ok {
		body.Text.Verbosity = responses.ResponseTextConfigVerbosity(verbosity)
	}

	return body, nil
}

// Creates a list of OpenAI Responses input items from a list of Horizon
// messages.
//
// NOTE: the `Name` field is not supported by Horizon.
func ResponsesMessages(ctx context.Context, in prompts.Prompts, attachments attachments.Attachments) (out responses.ResponseNewParamsInputUnion, err error) {
	var attachParts responses.ResponseInputMessageContentListParam
	if attachParts, err = ResponsesAttachments(ctx, attachments); err != nil {
		return out, err
	}

	in, attachmentIndex := attachmentMessages(in, len(attachments) > 0)
	items := make(responses.ResponseInputParam, 0, len(in))
	for i, msg := range in {
		if msg == nil {
			continue
		}
		var item responses.ResponseInputItemUnionParam
		switch msg.Role {
		case prompts.RoleTool:
			if len(msg.ToolResults) == 0 {
				return out, errors.ErrToolMessageMissingResults
			}
			for _, result := range msg.ToolResults {
				resultItem := responses.ResponseInputItemParamOfFunctionCallOutput(result.Content)
				resultItem.OfFunctionCallOutput.CallID = openai.String(result.CallID)
				items = append(items, resultItem)
			}
			continue
		case prompts.RoleAssistant:
			assistantItems, err := responsesAssistantItems(msg)
			if err != nil {
				return out, err
			}
			items = append(items, assistantItems...)
			continue
		case prompts.RoleUser:
			userParts := responses.ResponseInputMessageContentListParam{
				responses.ResponseInputContentParamOfInputText(msg.Content),
			}
			if i == attachmentIndex {
				userParts = append(userParts, attachParts...)
			}
			item = responses.ResponseInputItemParamOfMessage(userParts, responses.EasyInputMessageRoleUser)
		case prompts.RoleSystem:
			item = responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleSystem)
		case prompts.RoleDeveloper:
			item = responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleDeveloper)
		default:
			return out, fmt.Errorf("%w: %s", errors.ErrUnsupportedMessageRole, msg.Role)
		}

		if msg.Phase != "" {
			item.OfMessage.Phase = responses.EasyInputMessagePhase(msg.Phase)
		}
		items = append(items, item)
	}

	out.OfInputItemList = items
	return out, nil
}

// Replays one assistant prompt as ordered Responses items. Reasoning is opaque
// continuation data; function call IDs are distinct from provider item IDs.
func responsesAssistantItems(msg *prompts.Prompt) (responses.ResponseInputParam, error) {
	// Responses reasoning items must be replayed with their encrypted content when
	// the adapter manages continuation through the prompt transcript, so replay
	// the exact content that we saved previously, if present.
	if raw, ok := msg.Meta[responsesReasoningItemMetaKey].(string); ok {
		var reasoning responses.ResponseReasoningItemParam
		if err := json.Unmarshal([]byte(raw), &reasoning); err != nil {
			return nil, fmt.Errorf("decode Responses reasoning item: %w", err)
		}
		return responses.ResponseInputParam{{OfReasoning: &reasoning}}, nil
	}

	items := make(responses.ResponseInputParam, 0, len(msg.ToolCalls)+1)

	// A call-only prompt does not create an additional empty assistant message.
	if msg.Content != "" || len(msg.ToolCalls) == 0 {
		items = append(items, responsesAssistantMessage(msg))
	}

	for _, call := range msg.ToolCalls {
		item := responses.ResponseInputItemParamOfFunctionCall(call.Arguments, call.CallID, call.Name)
		if msg.ID != "" {
			item.OfFunctionCall.ID = param.NewOpt(msg.ID)
		}
		if msg.Status != "" {
			item.OfFunctionCall.Status = responses.ResponseFunctionToolCallStatus(msg.Status)
		}
		items = append(items, item)
	}
	return items, nil
}

// Provider output messages retain their item identity, status, phase, and
// refusal content. Caller-authored assistant messages use the simple input form.
func responsesAssistantMessage(msg *prompts.Prompt) responses.ResponseInputItemUnionParam {
	if msg.ID == "" {
		item := responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant)
		if msg.Phase != "" {
			item.OfMessage.Phase = responses.EasyInputMessagePhase(msg.Phase)
		}
		return item
	}

	content := make([]responses.ResponseOutputMessageContentUnionParam, 0, 2)
	refusal, hasRefusal := msg.Meta["refusal"].(string)
	if msg.Content != "" && (msg.Type != prompts.TypeRefusal || hasRefusal) {
		content = append(content, responses.ResponseOutputMessageContentUnionParam{
			OfOutputText: &responses.ResponseOutputTextParam{
				Text:        msg.Content,
				Annotations: []responses.ResponseOutputTextAnnotationUnionParam{},
			},
		})
	}
	if msg.Type == prompts.TypeRefusal {
		if !hasRefusal {
			refusal = msg.Content
		}
		content = append(content, responses.ResponseOutputMessageContentUnionParam{
			OfRefusal: &responses.ResponseOutputRefusalParam{Refusal: refusal},
		})
	}
	item := responses.ResponseInputItemParamOfOutputMessage(content, msg.ID, responses.ResponseOutputMessageStatus(msg.Status))
	if msg.Phase != "" {
		item.OfOutputMessage.Phase = responses.ResponseOutputMessagePhase(msg.Phase)
	}
	return item
}

// ResponsesTools maps provider-neutral tool definitions to Responses function
// tools.
func ResponsesTools(definitions []capabilities.Definition) ([]responses.ToolUnionParam, error) {
	if len(definitions) == 0 {
		return nil, nil
	}

	tools := make([]responses.ToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		parameters, err := definition.InputSchema.Object()
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", definition.Name, err)
		}
		strict := definition.InputSchema != nil && definition.InputSchema.Strict
		tool := responses.ToolParamOfFunction(string(definition.Name), parameters, strict)
		tool.OfFunction.Description = openai.String(definition.Description)
		tools = append(tools, tool)
	}
	return tools, nil
}

// Creates an OpenAI Responses response format from a Horizon schema. The
// response format can be a nil schema, text/plain, application/json, or
// application/schema+json. A schema is only added to the request if the mime
// type is application/schema+json.
func ResponsesResponseFormat(schema *schema.Schema) (format responses.ResponseFormatTextConfigUnionParam, err error) {
	// If the schema is nil, or plain text is requested, return text output format by default.
	if schema == nil || schema.MimeType.IsUnknown() || schema.MimeType == mime.TextPlain {
		return responses.ResponseFormatTextConfigUnionParam{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	}

	// Switch on the mime type of the schema
	switch schema.MimeType {
	case mime.ApplicationJSON:
		return responses.ResponseFormatTextConfigUnionParam{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}, nil
	case mime.ApplicationSchemaJSON:
		var schemaMap map[string]any
		if schemaMap, err = schema.Object(); err != nil {
			return format, err
		}

		return responses.ResponseFormatTextConfigUnionParam{
			OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
				// The name of the response format. Must be a-z, A-Z, 0-9, or contain underscores
				// and dashes, with a maximum length of 64.
				Name: schema.Name,

				// The schema for the response format, described as a JSON Schema object.
				Schema: schemaMap,

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
		}, nil
	default:
		return format, fmt.Errorf("%w: %s", errors.ErrUnsupportedOutputSchemaMimeType, schema.MimeType)
	}
}

// Converts the Responses API status (and incomplete reason) to a Horizon output type.
func ResponseStatusType(status responses.ResponseStatus, incompleteReason string) prompts.Type {
	switch status {
	case responses.ResponseStatusCompleted:
		return prompts.TypeMessage
	case responses.ResponseStatusIncomplete:
		if incompleteReason == "content_filter" {
			return prompts.TypeContentFilter
		}
		return prompts.TypeMessage
	default:
		return prompts.TypeUnknown
	}
}

// Converts Horizon attachments to Responses content parts, using ctx for remote
// attachment downloads.
func ResponsesAttachments(ctx context.Context, attachments attachments.Attachments) (parts responses.ResponseInputMessageContentListParam, err error) {
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
			parts = append(parts, responses.ResponseInputContentParamOfInputText(text))
		case "image":
			if uri, err = attachment.Base64URI(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					Detail:   responses.ResponseInputImageDetailAuto,
					ImageURL: openai.String(uri),
				},
			})
		case "audio":
			// TODO: Needs further testing since there are some inconsistencies with
			// how audio files are handled across different model providers.
			// Responses message content is still input_text, input_image, and
			// input_file only (no input_audio). Audio is sent as an input_file.
			if uri, err = attachment.Base64URI(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputFile: &responses.ResponseInputFileParam{
					FileData: openai.String(uri),
					Filename: openai.String(attachment.Filename),
				},
			})
		default:
			if uri, err = attachment.Base64URI(ctx); err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputFile: &responses.ResponseInputFileParam{
					FileData: openai.String(uri),
					Filename: openai.String(attachment.Filename),
				},
			})
		}
	}
	return parts, nil
}
