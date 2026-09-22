package api

//go:generate enumify -names outputTypeNames -no-tests

// Supported output types for LLM responses, such as "message", "refusal",
// "tool_call", etc.
type OutputType uint8

const (
	OutputUnknown OutputType = iota
	OutputMessage
	OutputRefusal
	OutputContentFilter
	OutputReasoning
	OutputCompaction
	OutputImageGeneration
	OutputMCPCall
	OutputMCPListTools
	OutputMCPApprovalRequest
	OutputMCPApprovalResponse
	OutputToolCall
	OutputToolCallResult
	OutputToolSearchCall
	OutputToolSearchResult
	OutputFileSearchCall
	OutputFunctionCall
	OutputFunctionCallResult
	OutputWebSearchCall
	OutputComputerCall
	OutputComputerCallResult
	OutputCodeInterpreterCall
	OutputLocalShellCall
	OutputLocalShellResult
	OutputShellCall
	OutputShellResult
	OutputApplyPatchCall
	OutputApplyPatchResult
)

var outputTypeNames = []string{
	"unknown",
	"message",
	"refusal",
	"content_filter",
	"reasoning",
	"compaction",
	"image_generation_call",
	"mcp_call",
	"mcp_list_tools",
	"mcp_approval_request",
	"mcp_approval_response",
	"custom_tool_call",
	"custom_tool_call_output",
	"tool_search_call",
	"tool_search_output",
	"file_search_call",
	"function_call",
	"function_call_output",
	"web_search_call",
	"computer_call",
	"computer_call_output",
	"code_interpreter_call",
	"local_shell_call",
	"local_shell_call_output",
	"shell_call",
	"shell_call_output",
	"apply_patch_call",
	"apply_patch_call_output",
}
