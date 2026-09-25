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
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"go.rtnl.ai/endeavor/pkg/horizon"
	"go.rtnl.ai/endeavor/pkg/horizon/capabilities"
	"go.rtnl.ai/endeavor/pkg/horizon/client/config"
	"go.rtnl.ai/endeavor/pkg/horizon/media"
)

var (
	ErrNoResponseOutput = errors.New("no response output items returned")
)

// Client for the OpenAI Responses API.
type ResponsesClient struct {
	client *openai.Client
}

// Creates a new [ResponsesClient] from the given client configuration.
func NewResponses(conf config.Provider) (rc *ResponsesClient, err error) {
	rc = &ResponsesClient{}
	if rc.client, err = New(conf); err != nil {
		return nil, err
	}
	return rc, nil
}

// Generate generates a response from the OpenAI responses API.
func (rc *ResponsesClient) Generate(ctx context.Context, req *horizon.Request) (out *horizon.Response, err error) {
	// Create the request body to send via the OpenAI responses API.
	var body responses.ResponseNewParams
	if body, err = ResponsesBody(req); err != nil {
		return nil, err
	}

	// Execute the responses API request.
	var resp *responses.Response
	if resp, err = rc.client.Responses.New(ctx, body, option.WithRequestTimeout(config.DefaultTimeout)); err != nil {
		// TODO: handle errors in a standardized way
		return nil, err
	}

	if resp.Error.Message != "" {
		return nil, fmt.Errorf("openai responses error: %s: %s", resp.Error.Code, resp.Error.Message)
	}

	if len(resp.Output) == 0 {
		return nil, ErrNoResponseOutput
	}

	// TODO: Better handling of the Usage data, just collecting simple info right now.
	out = &horizon.Response{
		ID:    resp.ID,
		Model: string(resp.Model),
		Usage: horizon.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
		Created: time.Unix(int64(resp.CreatedAt), 0),
		Meta:    make(map[string]any),
	}

	// Include API cost if provided
	// TODO: The OpenAI Go SDK isn't properly reporting fields as valid, so we have to
	// do the valid checking manually.
	if cost, ok := resp.Usage.JSON.ExtraFields["cost"]; ok && cost.Raw() != "" {
		if out.Usage.APICost, err = strconv.ParseFloat(cost.Raw(), 64); err != nil {
			return nil, err
		}
	}

	// Update the metadata of the response.
	out.Meta.PutString("object", resp.Object)
	out.Meta.PutString("service_tier", resp.ServiceTier)
	out.Meta.PutString("status", resp.Status)
	if resp.IncompleteDetails.Reason != "" {
		out.Meta.PutString("incomplete_reason", resp.IncompleteDetails.Reason)
	}

	for _, item := range resp.Output {
		switch variant := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			msg := horizon.Output{
				ID:     variant.ID,
				Role:   horizon.RoleAssistant, // Always assistant role for response messages.
				Status: string(variant.Status),
				Type:   ResponseStatusType(resp.Status, resp.IncompleteDetails.Reason),
				Meta:   make(horizon.Meta),
			}

			msg.Meta.PutString("status", variant.Status)
			msg.Meta.PutString("phase", variant.Phase)

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
					if msg.Type == horizon.OutputUnknown || msg.Type == horizon.OutputMessage {
						msg.Type = horizon.OutputRefusal
					}
				}
			}

			// TODO: Don't add empty logprobs to metadata.
			if len(logprobs) > 0 {
				msg.Meta["logprobs"] = logprobs
			}

			// Handle annotations on the message. Responses can include several
			// annotation types; Horizon currently only maps URL citations.
			if len(annotations) > 0 {
				msg.Citations = make([]horizon.Citation, 0, len(annotations))
				for _, annotation := range annotations {
					if annotation.Type != "url_citation" {
						continue
					}
					msg.Citations = append(msg.Citations, horizon.Citation{
						StartIndex: annotation.StartIndex,
						EndIndex:   annotation.EndIndex,
						Title:      annotation.Title,
						URL:        annotation.URL,
					})
				}
			}

			out.Output = append(out.Output, msg)
		case responses.ResponseFunctionToolCall:
			// Convert chat completion tool calls to Horizon tool calls.
			out.ToolCalls = append(out.ToolCalls, capabilities.ToolCall{
				CallID:    variant.CallID,
				Name:      variant.Name,
				Arguments: json.RawMessage(variant.Arguments),
			})
		default:
			// TODO: Handle other output item types (reasoning, etc.)
		}
	}

	// Fall back to aggregated text if the response had output items but none were
	// mapped as messages (for example, providers that only populate output_text).
	if len(out.Output) == 0 && len(out.ToolCalls) == 0 {
		if text := resp.OutputText(); text != "" {
			out.Output = append(out.Output, horizon.Output{
				Role:    horizon.RoleAssistant,
				Type:    ResponseStatusType(resp.Status, resp.IncompleteDetails.Reason),
				Content: text,
				Meta:    make(horizon.Meta),
			})
		} else {
			return nil, ErrNoResponseOutput
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
func ResponsesBody(req *horizon.Request) (body responses.ResponseNewParams, err error) {
	body = responses.ResponseNewParams{
		Background: openai.Bool(false),
		Store:      openai.Bool(false),
		Model:      req.Model,
	}

	if body.Tools, err = ResponsesTools(req.Tools); err != nil {
		return body, err
	}
	if len(req.Tools) > 0 {
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
		body.ParallelToolCalls = openai.Bool(true)
	}

	if body.Input, err = ResponsesMessages(req.Input, req.Attachments); err != nil {
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
func ResponsesMessages(in []horizon.Message, attachments []*horizon.Attachment) (out responses.ResponseNewParamsInputUnion, err error) {
	// Handle the attachment parts
	var parts responses.ResponseInputMessageContentListParam
	if parts, err = ResponsesAttachments(attachments); err != nil {
		return out, err
	}

	items := make(responses.ResponseInputParam, 0, len(in))
	for _, msg := range in {
		if msg.Role == horizon.RoleTool {
			for _, result := range msg.ToolResults {
				item := responses.ResponseInputItemParamOfFunctionCallOutput(toolResultText(result))
				item.OfFunctionCallOutput.CallID = openai.String(result.CallID)

				items = append(items, item)
			}

			if len(msg.ToolResults) > 0 {
				continue
			}
		}

		if msg.Role == horizon.RoleAssistant && len(msg.ToolCalls) > 0 {
			if msg.Content != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(
					msg.Content,
					responses.EasyInputMessageRoleAssistant,
				))
			}
			for _, call := range msg.ToolCalls {
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(
					string(call.Arguments),
					call.CallID,
					call.Name,
				))
			}
			continue
		}

		var role responses.EasyInputMessageRole
		switch msg.Role {
		case horizon.RoleUser:
			role = responses.EasyInputMessageRoleUser
		case horizon.RoleAssistant:
			role = responses.EasyInputMessageRoleAssistant
		case horizon.RoleSystem:
			role = responses.EasyInputMessageRoleSystem
		case horizon.RoleDeveloper:
			role = responses.EasyInputMessageRoleDeveloper
		default:
			return out, fmt.Errorf("unsupported message role: %s", msg.Role)
		}

		var item responses.ResponseInputItemUnionParam
		if role == responses.EasyInputMessageRoleUser {
			userParts := append(responses.ResponseInputMessageContentListParam{
				responses.ResponseInputContentParamOfInputText(msg.Content),
			}, parts...)
			item = responses.ResponseInputItemParamOfMessage(userParts, role)
		} else {
			item = responses.ResponseInputItemParamOfMessage(msg.Content, role)
		}
		if msg.Phase != "" {
			item.OfMessage.Phase = responses.EasyInputMessagePhase(msg.Phase)
		}
		items = append(items, item)
	}

	out.OfInputItemList = items
	return out, nil
}

// ResponsesTools maps provider-neutral tool definitions to Responses function
// tools.
func ResponsesTools(definitions []capabilities.ToolDefinition) ([]responses.ToolUnionParam, error) {
	if len(definitions) == 0 {
		return nil, nil
	}

	tools := make([]responses.ToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		parameters, err := toolParameters(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", definition.Name, err)
		}
		tool := responses.ToolParamOfFunction(definition.Name, parameters, false)
		tool.OfFunction.Description = openai.String(definition.Description)
		if outputSchema, err := toolSchema(definition.OutputSchema); err != nil {
			return nil, fmt.Errorf("tool %q output schema: %w", definition.Name, err)
		} else {
			tool.OfFunction.OutputSchema = outputSchema
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// Creates an OpenAI Responses response format from a Horizon schema. The
// response format can be a nil schema, text/plain, application/json, or
// application/schema+json. A schema is only added to the request if the mime
// type is application/schema+json.
func ResponsesResponseFormat(schema *horizon.Schema) (format responses.ResponseFormatTextConfigUnionParam, err error) {
	// If the schema is nil, or plain text is requested, return text output format by default.
	if schema == nil || schema.MimeType.IsUnknown() || schema.MimeType == media.TextPlain {
		return responses.ResponseFormatTextConfigUnionParam{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	}

	// Switch on the mime type of the schema
	switch schema.MimeType {
	case media.ApplicationJSON:
		return responses.ResponseFormatTextConfigUnionParam{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}, nil
	case media.ApplicationSchemaJSON:
		var schemaMap map[string]any
		if schemaMap, err = jsonSchemaObject(schema); err != nil {
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
		return format, fmt.Errorf("unsupported output schema mime type: %s", schema.MimeType)
	}
}

// Converts the Responses API status (and incomplete reason) to a Horizon output type.
func ResponseStatusType(status responses.ResponseStatus, incompleteReason string) horizon.OutputType {
	switch status {
	case responses.ResponseStatusCompleted:
		return horizon.OutputMessage
	case responses.ResponseStatusIncomplete:
		if incompleteReason == "content_filter" {
			return horizon.OutputContentFilter
		}
		return horizon.OutputMessage
	default:
		return horizon.OutputUnknown
	}
}

// Convert Horizon attachments to OpenAI Responses content parts.
func ResponsesAttachments(attachments []*horizon.Attachment) (parts responses.ResponseInputMessageContentListParam, err error) {
	var uri string
	for _, attachment := range attachments {
		switch attachment.MimeType.Top() {
		case "text":
			var text string
			if text, err = attachment.Text(); err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentParamOfInputText(text))
		case "image":
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					Detail:   responses.ResponseInputImageDetailAuto,
					ImageURL: openai.String(attachment.URL),
				},
			})
		case "audio":
			// TODO: Needs further testing since there are some inconsistencies with
			// how audio files are handled across different model providers.
			// Responses message content is still input_text, input_image, and
			// input_file only (no input_audio). Audio is sent as an input_file.
			if uri, err = attachment.Base64URI(); err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputFile: &responses.ResponseInputFileParam{
					FileData: openai.String(uri),
					Filename: openai.String(attachment.Filename),
				},
			})
		default:
			if uri, err = attachment.Base64URI(); err != nil {
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

// Converts a Horizon schema into a JSON Schema object map for the Responses API.
func jsonSchemaObject(schema *horizon.Schema) (_ map[string]any, err error) {
	var raw any
	if raw, err = schema.JSONSchema(); err != nil {
		return nil, err
	}

	switch v := raw.(type) {
	case map[string]any:
		return v, nil
	case json.RawMessage:
		var out map[string]any
		if err = json.Unmarshal(v, &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		var data []byte
		if data, err = json.Marshal(v); err != nil {
			return nil, err
		}
		var out map[string]any
		if err = json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}
