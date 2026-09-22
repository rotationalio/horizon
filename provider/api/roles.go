package api

import "go.rtnl.ai/horizon/prompts"

// Role is retained as a compatibility alias for the shared prompt role type.
type Role = prompts.Role

const (
	RoleUnknown   = prompts.RoleUnknown
	RoleUser      = prompts.RoleUser
	RoleSystem    = prompts.RoleSystem
	RoleDeveloper = prompts.RoleDeveloper
	RoleAssistant = prompts.RoleAssistant
	RoleTool      = prompts.RoleTool
	RoleFunction  = prompts.RoleFunction
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

// ParseRole parses a shared prompt role.
func ParseRole(value any) (Role, error) {
	return prompts.ParseRole(value)
}
