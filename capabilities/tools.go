package capabilities

import (
	"context"
	"encoding/json"

	"go.rtnl.ai/ulid"
)

// FIXME: refactor the capability/tools provider interfaces (in ticket 409760 and 40977)

// ToolsProvider lists the tools available to a task and invokes an advertised tool.
type ToolsProvider interface {
	ListTools(context.Context) ([]ToolDefinition, error)
	InvokeTool(context.Context, ToolCall) (ToolResult, error)
}

// ToolDefinition describes a tool that can be advertised to a model.
type ToolDefinition struct {
	ToolID        ulid.ULID
	IntegrationID ulid.ULID
	Name          string
	Description   string
	InputSchema   json.RawMessage
	OutputSchema  json.RawMessage
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	CallID        string
	ToolID        ulid.ULID
	IntegrationID ulid.ULID
	Name          string
	Arguments     json.RawMessage
}

// ToolResult is the provider-independent result of a tool invocation. Content can be
// multimodal, so the caller must handle it based on the content type.
type ToolResult struct {
	CallID   string
	Content  []Content
	IsError  bool
	Metadata map[string]any
}
