package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/x/mime"
)

// Generate performs one inference request.
func (p *process) Generate(ctx context.Context) error {
	// Run input guard if implemented.
	if guard, ok := p.runner.(task.InputGuard); ok {
		if err := cancelAndTrace(ctx, "guard.input", func(context.Context) error {
			return guard.ProtectInput(p.request)
		}); err != nil {
			return fmt.Errorf("protect inference input: %w", err)
		}
	}

	// Perform generation inside its own span.
	p.output.Usage.Invocations++
	requestCtx, span := tracer.Start(ctx, "provider.generate")
	response, generateErr := p.provider.Generate(requestCtx, p.request)
	span.AddEvent("provider.response", trace.WithAttributes(attribute.Bool("provider.response_present", response != nil)))
	if generateErr != nil {
		span.SetStatus(codes.Error, "provider request failed")
	}
	span.End()

	// A returned response means the provider call completed; keep its accounting
	// even when the caller canceled before Horizon could deliver the content.
	if response != nil {
		p.recordUsage(response.Usage)
	}

	if ctx.Err() != nil {
		// Do not run output guards or capture content after cancellation. The
		// enclosing cancelAndTrace adds the caller's cancellation to this error.
		if generateErr != nil {
			return fmt.Errorf("provider inference failed: %w", generateErr)
		}
		return nil
	}

	var responseErr error
	if response != nil {
		// Run the output guard before capturing the response if implemented.
		if guard, ok := p.runner.(task.OutputGuard); ok {
			if err := cancelAndTrace(ctx, "guard.output", func(context.Context) error {
				return guard.ProtectOutput(response)
			}); err != nil {
				responseErr = fmt.Errorf("protect inference output: %w", err)
			}
		}

		// Capture the response.
		if responseErr == nil {
			if err := p.captureResponse(response); err != nil {
				responseErr = fmt.Errorf("process inference response: %w", err)
			}
		}
	} else if generateErr == nil {
		responseErr = errors.ErrNoModelOutput
	}

	// Preserve both provider generation failures and response-guard/decoding
	// failures.
	if generateErr != nil {
		generateErr = fmt.Errorf("provider inference failed: %w", generateErr)
	}
	return errors.Join(responseErr, generateErr)
}

// Adds the usage from a provider response onto the output usage.
func (p *process) recordUsage(usage provider.Usage) {
	p.output.Usage.InputTokens += usage.InputTokens
	p.output.Usage.OutputTokens += usage.OutputTokens
	p.output.Usage.TotalTokens += usage.TotalTokens
	p.output.Usage.APICost += usage.APICost
}

// Captures a non-nil provider response and stores it in the output.
func (p *process) captureResponse(response *provider.Response) error {

	// Use the provider's resolved model when available, otherwise keep the
	// requested slug.
	model := response.Model
	if model == "" {
		model = p.task.Model.Slug
	}
	p.output.Model = task.Model{Slug: model, Parameters: p.task.Model.Parameters}

	// Store the response attachments.
	p.output.Attachments = append(p.output.Attachments, response.Attachments...)

	// Only assistant text is task output. Tool calls belong to the separate capabilities loop.
	var finalContent, unphasedContent []string
	for _, message := range response.Output {
		if message == nil || message.Role != prompts.RoleAssistant {
			continue
		}

		if message.Type == prompts.TypeToolCall || len(message.ToolCalls) > 0 {
			// TODO: handle tool calls
			return errors.ErrCapabilityProviderRequired
		}

		if message.Content == "" {
			continue
		}

		switch message.Phase {
		case "final_answer":
			finalContent = append(finalContent, message.Content)
		case "":
			unphasedContent = append(unphasedContent, message.Content)
		}
	}

	// Prefer explicitly final text; use unphased assistant text as a fallback.
	content := finalContent
	if len(content) == 0 {
		content = unphasedContent
	}

	if len(content) == 0 {
		// An attachment-only response is still a usable result.
		if len(response.Attachments) > 0 {
			return nil
		}
		return errors.ErrNoModelOutput
	}

	text := strings.Join(content, "\n")

	// Without a declared output MIME type, expose generated content as plain text.
	if p.task.Output.Schema == nil || p.task.Output.Schema.MimeType.IsUnknown() {
		p.output.MimeType = mime.TextPlain
		p.output.Output = text
		return nil
	}

	switch p.task.Output.Schema.MimeType {
	case mime.ApplicationJSON, mime.ApplicationSchemaJSON:
		// TODO: Normalize model output before parsing, including JSON wrapped in Markdown code fences.
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			// Return the raw text as-is if the JSON is invalid so that the
			// caller may debug if necessary.
			p.output.Output = text
			return fmt.Errorf("%w: invalid JSON response: %w", errors.ErrInvalidModelOutput, err)
		}
		// TODO: Validate the JSON result against the task's declared JSON Schema, not just its syntax.
		p.output.Output = raw
		p.output.MimeType = mime.ApplicationJSON
	case mime.TextPlain:
		p.output.Output = text
		p.output.MimeType = mime.TextPlain
	default:
		// Preserve the task's declared MIME type for other text-based formats.
		p.output.Output = text
		p.output.MimeType = p.task.Output.Schema.MimeType
	}
	return nil
}
