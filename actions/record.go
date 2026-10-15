package actions

import (
	"context"
	"strconv"
	"strings"

	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
)

// AddMessage records standalone conversation messages, retaining their roles.
func (a *Actions) AddMessage(ctx context.Context, sender, receiver Entity, messages prompts.Prompts, options ...Option) uint64 {
	return a.record(ctx, &Action{
		Type:     TypeMessage,
		Sender:   sender,
		Receiver: receiver,
		Messages: messages,
	}, options)
}

// AddGenerationRequest returns a correlation ID to pass to AddGenerationResponse.
// Generation numbers are local to the log and continue after JSON decoding.
func (a *Actions) AddGenerationRequest(ctx context.Context, request *provider.Request, options ...Option) string {
	event := &Action{
		Type:              TypeGenerationRequest,
		Sender:            EntityRunner,
		Receiver:          EntityModel,
		GenerationRequest: request,
	}
	applyOptions(event, options)

	// Correlation and sequence are allocated in the same append critical section.
	_, correlation := a.appendEvent(ctx, event, true)
	return correlation
}

// AddGenerationResponse records a provider result, including errors or partial
// output. Record delivered messages separately with AddMessage.
func (a *Actions) AddGenerationResponse(ctx context.Context, correlation string, response *provider.Response, err error, options ...Option) uint64 {
	return a.record(ctx, &Action{
		Type:               TypeGenerationResponse,
		CorrelationID:      correlation,
		Sender:             EntityModel,
		Receiver:           EntityRunner,
		GenerationResponse: response,
		Error:              errorText(err),
	}, options)
}

// AddCapabilityRequest records a dispatch or attempted dispatch. Model tool-call
// IDs correlate requests/results; the capability name remains in the payload.
func (a *Actions) AddCapabilityRequest(ctx context.Context, call prompts.ToolCall, options ...Option) uint64 {
	return a.record(ctx, &Action{
		Type:              TypeCapabilityRequest,
		CorrelationID:     call.CallID,
		Sender:            EntityRunner,
		Receiver:          EntityCapability,
		CapabilityRequest: &call,
	}, options)
}

// AddCapabilityResponse records a tool result or failure for debugging.
// It never contributes transcript messages; record delivery with AddMessage.
func (a *Actions) AddCapabilityResponse(ctx context.Context, callID string, result *prompts.ToolResponse, err error, options ...Option) uint64 {
	// The request owns correlation; do not let an executor replace its call ID.
	if result != nil {
		copy := *result
		copy.CallID = callID
		result = &copy
	}

	return a.record(ctx, &Action{
		Type:               TypeCapabilityResponse,
		CorrelationID:      callID,
		Sender:             EntityCapability,
		Receiver:           EntityRunner,
		CapabilityResponse: result,
		Error:              errorText(err),
	}, options)
}

// AddInputGuard records a validation decision. Input guard mutations are ignored
// by Horizon, so this event does not contribute messages to the conversation.
func (a *Actions) AddInputGuard(ctx context.Context, err error, options ...Option) uint64 {
	return a.record(ctx, &Action{
		Type:     TypeInputGuard,
		Sender:   EntityInputGuard,
		Receiver: EntityRunner,
		Error:    errorText(err),
	}, options)
}

// AddOutputGuard records a decision, correlated with its generation.
// Record accepted messages separately with AddMessage.
func (a *Actions) AddOutputGuard(ctx context.Context, correlation string, err error, options ...Option) uint64 {
	return a.record(ctx, &Action{
		Type:          TypeOutputGuard,
		CorrelationID: correlation,
		Sender:        EntityOutputGuard,
		Receiver:      EntityRunner,
		Error:         errorText(err),
	}, options)
}

// record adds an event to the log, applying any provided options first.
func (a *Actions) record(ctx context.Context, event *Action, options []Option) uint64 {
	applyOptions(event, options)
	return a.Add(ctx, event)
}

// observeGeneration restores the generation counter from decoded correlations.
func (a *Actions) observeGeneration(correlation string) {
	if !strings.HasPrefix(correlation, "generation-") {
		return
	}

	if number, err := strconv.ParseUint(strings.TrimPrefix(correlation, "generation-"), 10, 64); err == nil && number > a.generation {
		a.generation = number
	}
}

// errorText returns the error message or empty string if the error is nil.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
