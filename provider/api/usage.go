package api

// Usage is the model and capability usage information for a provider response.
type Usage struct {
	Invocations  uint64
	ToolCalls    uint64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	APICost      float64
	EnergyCost   float64
}
