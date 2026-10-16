package actions

//go:generate enumify -names entityNames

// Entity identifies the participant sending or receiving an action.
type Entity uint8

const (
	EntityUnknown Entity = iota
	EntitySystem
	EntityUser
	EntityModel
	EntityCapability
	EntityRunner
	EntityInputGuard
	EntityOutputGuard
)

var entityNames = []string{
	"unknown",
	"system",
	"user",
	"model",
	"capability",
	"runner",
	"input_guard",
	"output_guard",
}
