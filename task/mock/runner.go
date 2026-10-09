// Package mock provides callback-based test doubles for task.Runner and its optional
// interfaces. Each optional interface has its own mock type, so tests can expose only
// the interfaces they need by embedding those mocks together with Runner.
//
// Each mock embeds Horizon's call tracker, so tests can use Calls, AssertCalled,
// AssertNotCalled, and Reset on each component independently.
//
// For example, a runner that implements the required lifecycle and input processing
// interfaces can be composed inline:
//
//	runner := struct {
//		*mock.Runner
//		*mock.InputProcessor
//	}{
//		Runner: &mock.Runner{
//			OnPrepare:  func(context.Context, *task.Task) error { return nil },
//			OnFinalize: func(context.Context, *task.Output, error) error { return nil },
//		},
//		InputProcessor: &mock.InputProcessor{
//			OnProcessInput: func(input *task.Input) (*task.Input, error) {
//				return input, nil
//			},
//		},
//	}
//
// The resulting example satisfies task.Runner and task.InputProcessor, but not the
// other optional runner interfaces. Go fixes a type's method set at compile time, so
// this explicit composition keeps each test's interface combination clear.
package mock

import (
	"context"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/internal/mock"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
)

var (
	_ task.Runner              = (*Runner)(nil)
	_ task.InputProcessor      = (*InputProcessor)(nil)
	_ task.ContextProcessor    = (*ContextProcessor)(nil)
	_ task.AttachmentProcessor = (*AttachmentProcessor)(nil)
	_ task.Renderer            = (*Renderer)(nil)
	_ task.InputGuard          = (*InputGuard)(nil)
	_ task.OutputGuard         = (*OutputGuard)(nil)
	_ task.CapabilityRunner    = (*CapabilityRunner)(nil)
)

// Operation names are used with the embedded call tracker assertions.
const (
	Prepare            = "Prepare"
	Finalize           = "Finalize"
	ProcessInput       = "ProcessInput"
	ProcessContext     = "ProcessContext"
	ProcessAttachment  = "ProcessAttachment"
	Render             = "Render"
	ProtectInput       = "ProtectInput"
	ProtectOutput      = "ProtectOutput"
	LookupCapabilities = "LookupCapabilities"
	ExecuteCapability  = "ExecuteCapability"
)

// Runner mocks the required task.Runner lifecycle methods. Both callbacks must be set
// before execution; calling an unset callback panics.
type Runner struct {
	mock.Mock
	OnPrepare  func(ctx context.Context, task *task.Task) error
	OnFinalize func(ctx context.Context, output *task.Output, executionErr error) error
}

// Prepare invokes OnPrepare. It panics if OnPrepare is unset.
func (r *Runner) Prepare(ctx context.Context, task *task.Task) error {
	r.Call(Prepare)
	if r.OnPrepare == nil {
		panic("mock.Runner.OnPrepare callback is not set")
	}
	return r.OnPrepare(ctx, task)
}

// Finalize invokes OnFinalize. It panics if OnFinalize is unset.
func (r *Runner) Finalize(ctx context.Context, output *task.Output, executionErr error) error {
	r.Call(Finalize)
	if r.OnFinalize == nil {
		panic("mock.Runner.OnFinalize callback is not set")
	}
	return r.OnFinalize(ctx, output, executionErr)
}

// InputProcessor mocks task.InputProcessor. It panics if OnProcessInput is unset.
type InputProcessor struct {
	mock.Mock
	OnProcessInput func(*task.Input) (*task.Input, error)
}

// ProcessInput invokes OnProcessInput. It panics if OnProcessInput is unset.
func (m *InputProcessor) ProcessInput(input *task.Input) (*task.Input, error) {
	m.Call(ProcessInput)
	if m.OnProcessInput == nil {
		panic("mock.InputProcessor.OnProcessInput callback is not set")
	}
	return m.OnProcessInput(input)
}

// ContextProcessor mocks task.ContextProcessor. It panics if OnProcessContext is unset.
type ContextProcessor struct {
	mock.Mock
	OnProcessContext func(prompts.Context) (prompts.Context, error)
}

// ProcessContext invokes OnProcessContext. It panics if OnProcessContext is unset.
func (m *ContextProcessor) ProcessContext(ctx prompts.Context) (prompts.Context, error) {
	m.Call(ProcessContext)
	if m.OnProcessContext == nil {
		panic("mock.ContextProcessor.OnProcessContext callback is not set")
	}
	return m.OnProcessContext(ctx)
}

// AttachmentProcessor mocks task.AttachmentProcessor. It panics if OnProcessAttachment is unset.
type AttachmentProcessor struct {
	mock.Mock
	OnProcessAttachment func(*attachments.Attachment) (*attachments.Attachment, error)
}

// ProcessAttachment invokes OnProcessAttachment. It panics if OnProcessAttachment is unset.
func (m *AttachmentProcessor) ProcessAttachment(attachment *attachments.Attachment) (*attachments.Attachment, error) {
	m.Call(ProcessAttachment)
	if m.OnProcessAttachment == nil {
		panic("mock.AttachmentProcessor.OnProcessAttachment callback is not set")
	}
	return m.OnProcessAttachment(attachment)
}

// Renderer mocks task.Renderer. It panics if OnRender is unset.
type Renderer struct {
	mock.Mock
	OnRender func(prompts.Templates, prompts.Context) (prompts.Prompts, error)
}

// Render invokes OnRender with the supplied templates and context. It panics if OnRender is unset.
func (m *Renderer) Render(templates prompts.Templates, ctx prompts.Context) (prompts.Prompts, error) {
	m.Call(Render)
	if m.OnRender == nil {
		panic("mock.Renderer.OnRender callback is not set")
	}
	return m.OnRender(templates, ctx)
}

// InputGuard mocks task.InputGuard. It panics if OnProtectInput is unset.
type InputGuard struct {
	mock.Mock
	OnProtectInput func(*provider.Request) error
}

// ProtectInput invokes OnProtectInput. It panics if OnProtectInput is unset.
func (m *InputGuard) ProtectInput(request *provider.Request) error {
	m.Call(ProtectInput)
	if m.OnProtectInput == nil {
		panic("mock.InputGuard.OnProtectInput callback is not set")
	}
	return m.OnProtectInput(request)
}

// OutputGuard mocks task.OutputGuard. It panics if OnProtectOutput is unset.
type OutputGuard struct {
	mock.Mock
	OnProtectOutput func(*provider.Response) error
}

// ProtectOutput invokes OnProtectOutput. It panics if OnProtectOutput is unset.
func (m *OutputGuard) ProtectOutput(response *provider.Response) error {
	m.Call(ProtectOutput)
	if m.OnProtectOutput == nil {
		panic("mock.OutputGuard.OnProtectOutput callback is not set")
	}
	return m.OnProtectOutput(response)
}

// CapabilityRunner mocks task.CapabilityRunner. Set both callbacks before use; calling
// either method with its callback unset panics.
type CapabilityRunner struct {
	mock.Mock
	OnLookupCapabilities func(context.Context, []capabilities.Name) ([]capabilities.Definition, error)
	OnExecuteCapability  func(context.Context, capabilities.Request) (capabilities.Response, *capabilities.Error)
}

// LookupCapabilities invokes OnLookupCapabilities. It panics if the callback is unset.
func (m *CapabilityRunner) LookupCapabilities(ctx context.Context, names []capabilities.Name) ([]capabilities.Definition, error) {
	m.Call(LookupCapabilities)
	if m.OnLookupCapabilities == nil {
		panic("mock.CapabilityRunner.OnLookupCapabilities callback is not set")
	}
	return m.OnLookupCapabilities(ctx, names)
}

// ExecuteCapability invokes OnExecuteCapability. It panics if the callback is unset.
func (m *CapabilityRunner) ExecuteCapability(ctx context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
	m.Call(ExecuteCapability)
	if m.OnExecuteCapability == nil {
		panic("mock.CapabilityRunner.OnExecuteCapability callback is not set")
	}
	return m.OnExecuteCapability(ctx, request)
}
