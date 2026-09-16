package prompts

//go:generate enumify -names roleNames

// Supported message roles for LLM input and output.
type Role uint8

const (
	RoleUnknown Role = iota
	RoleUser
	RoleSystem
	RoleDeveloper
	RoleAssistant
	RoleTool
	RoleFunction
)

var roleNames = []string{
	"unknown",
	"user",
	"system",
	"developer",
	"assistant",
	"tool",
	"function",
}
