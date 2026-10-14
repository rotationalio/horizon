package actions

//go:generate enumify -names entityNames

// Entity identifies the kind of participant sending or receiving an action.
// Entity kinds are distinct from the roles of messages inside a model request.
type Entity uint8

const (
	EntityUnknown Entity = iota
	EntitySystem
	EntityUser
	EntityModel
	EntityCapability
)

var entityNames = []string{
	"unknown",
	"system",
	"user",
	"model",
	"capability",
}
