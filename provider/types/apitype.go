package types

//go:generate enumify -names apiTypeNames

// APIType is the Horizon API/request surface used for inference.
type APIType uint8

const (
	APITypeUnknown APIType = iota
	APITypeMock
	APITypeOpenAIResponses
	APITypeOpenAIChatCompletions
)

var apiTypeNames = [][]string{
	{
		"unknown",
		"mock",
		"oai_responses",
		"oai_completions",
	},
	{
		"Unknown",
		"Mock",
		"OpenAI Responses",
		"OpenAI Chat Completions",
	},
}

// Returns a human-friendly API type name.
func (a APIType) DisplayName() string {
	return apiTypeNames[1][a]
}
