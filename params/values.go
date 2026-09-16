package params

type ParameterType uint8

const (
	String ParameterType = iota
	Bool
	Int
	Float
	JSON
)

type Value struct {
	Type    ParameterType
	Min     float64
	Max     float64
	Options []string
}

var parameterValues = map[string]Value{
	FrequencyPenalty: {
		Type: Float,
		Min:  -2.0,
		Max:  2.0,
	},
	LogProbs: {
		Type: Bool,
	},
	MaxCompletionTokens: {
		Type: Int,
		Max:  1e6,
	},
	MaxTokens: {
		Type: Int,
		Max:  1e6,
	},
	PresencePenalty: {
		Type: Float,
	},
	Seed: {
		Type: Int,
		Max:  1e6,
	},
	Temperature: {
		Type: Float,
		Max:  2.0,
	},
	TopLogProbs: {
		Type: Int,
		Max:  20,
	},
	TopP: {
		Type: Float,
		Max:  1.0,
	},
	PromptCacheKey: {
		Type: String,
	},
	SafetyIdentifier: {
		Type: String,
	},
	LogitBias: {
		Type: JSON,
	},
	PromptCacheRetention: {
		Type: String,
	},
	ReasoningEffort: {
		Type: String,
		Options: []string{
			"none",
			"minimal",
			"low",
			"medium",
			"high",
			"xhigh",
		},
	},
	ServiceTier: {
		Type: String,
		Options: []string{
			"auto",
			"default",
			"flex",
			"scale",
			"priority",
		},
	},
	Verbosity: {
		Type: String,
		Options: []string{
			"low",
			"medium",
			"high",
		},
	},
	ToolChoice: {
		Type: String,
	},
	MaxOutputTokens: {
		Type: Int,
		Max:  1e6,
	},
	MaxToolCalls: {
		Type: Int,
		Max:  128,
	},
	MaxToolTurns: {
		Type: Int,
		Max:  128,
	},
	Truncation: {
		Type: String,
		Options: []string{
			"auto",
			"disabled",
		},
	},
	ReasoningSummary: {
		Type: String,
		Options: []string{
			"auto",
			"concise",
			"detailed",
		},
	},
}

// Parse value restrictions from the parameter string.
func ParameterValue(param string) (value Value) {
	value, _ = parameterValues[param]
	return value
}
