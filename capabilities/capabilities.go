package capabilities

import "encoding/json"

// FIXME: refactor the capability/tools provider interfaces (in ticket 409760 and 40977)

// Provider is the generic capability boundary returned to task
// consumers. It embeds the tool provider so callers can use List and Invoke
// directly. Prompts and resources can be added here when they have consumers.
type Provider interface {
	ToolsProvider

	// TODO: Add prompt and resource accessors when Horizon has consumers for them.
}

// Content is one provider-independent result content item.
type Content struct {
	// TODO: Define the final multimodal representation when Horizon consumes images, audio, and embedded resources.
	Type string
	Text string
	Data json.RawMessage
}
