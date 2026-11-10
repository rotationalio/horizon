package provider

import "go.rtnl.ai/horizon/modality"

//go:generate enumify -names apiTypeNames

// APIType is the Horizon API/request surface used for inference.
type APIType uint8

const (
	APITypeUnknown APIType = iota
	APITypeMock
	APITypeOpenAIResponses
	APITypeOpenAIChatCompletions

	// APITypeDecisions covers decision and extraction models; the provider picks
	// the concrete route per model.
	APITypeDecisions
)

var apiTypeNames = [][]string{
	{
		"unknown",
		"mock",
		"oai_responses",
		"oai_completions",
		"decisions",
	},
	{
		"Unknown",
		"Mock",
		"OpenAI Responses",
		"OpenAI Chat Completions",
		"Decisions",
	},
}

var inputModalityMap = map[APIType]modality.Modality{
	APITypeUnknown:               modality.ZeroModality,
	APITypeMock:                  modality.ZeroModality,
	APITypeOpenAIResponses:       modality.Text | modality.Image | modality.Audio | modality.Video | modality.Document,
	APITypeOpenAIChatCompletions: modality.Text | modality.Image | modality.Audio | modality.Video | modality.Document,
	APITypeDecisions:             modality.Text, // Text only for now; image, audio and video are not wired up yet.
}

var outputModalityMap = map[APIType]modality.Modality{
	APITypeUnknown:               modality.ZeroModality,
	APITypeMock:                  modality.ZeroModality,
	APITypeOpenAIResponses:       modality.Text | modality.Image | modality.Audio | modality.Video | modality.Document,
	APITypeOpenAIChatCompletions: modality.Text | modality.Audio,
	APITypeDecisions:             modality.Decisions,
}

// Returns a human-friendly API type name.
func (a APIType) DisplayName() string {
	return apiTypeNames[1][a]
}

// Returns the input modality for the API type.
func (a APIType) InputModality() modality.Modality {
	return inputModalityMap[a]
}

// Returns the output modality for the API type.
func (a APIType) OutputModality() modality.Modality {
	return outputModalityMap[a]
}
