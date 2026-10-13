package horizon_test

//=============================================================================
// Shared runner and provider-response fixtures for the capabilities tests.
// Individual tests customize these fixtures to script their own execution flow.
//=============================================================================

import (
	"context"

	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
)

// Bundles the runner, capability executor, and guard mocks used by these tests. Embedding
// the individual mocks lets each test expose only the optional interfaces it needs.
type capabilityTestRunner struct {
	*mock.Runner
	*mock.CapabilityRunner
	*mock.InputGuard
	*mock.OutputGuard
}

// Creates a runner fixture with definitions for the selected names, no-op guards, and a
// default text result. Individual tests replace only the callbacks needed for their case.
func newCapabilityTestRunner(names ...capabilities.Name) *capabilityTestRunner {
	return &capabilityTestRunner{
		Runner: &mock.Runner{
			OnPrepare:  func(_ context.Context, definition *task.Task) error { definition.Capabilities = names; return nil },
			OnFinalize: func(context.Context, *task.Output, error) error { return nil },
		},
		CapabilityRunner: &mock.CapabilityRunner{
			OnLookupCapabilities: func(_ context.Context, selected []capabilities.Name) ([]capabilities.Definition, error) {
				definitions := make([]capabilities.Definition, 0, len(selected))
				for _, name := range selected {
					definitions = append(definitions, capabilities.Definition{Name: name})
				}
				return definitions, nil
			},
			OnExecuteCapability: func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
				return capabilities.NewToolTextContentResponse("done"), nil
			},
		},
		InputGuard:  &mock.InputGuard{OnProtectInput: func(*provider.Request) error { return nil }},
		OutputGuard: &mock.OutputGuard{OnProtectOutput: func(*provider.Response) error { return nil }},
	}
}

// Builds a provider response containing assistant tool calls and the same usage totals as
// the generation fixtures, so loop tests can focus on call handling and conversation state.
func capabilityTestResponse(calls ...prompts.ToolCall) *provider.Response {
	response := processTestResponse()
	response.Output = prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeToolCall, Content: "intermediate text", ToolCalls: calls}}
	return response
}

func capabilityTurns(turns int64) *int64 { return &turns }
