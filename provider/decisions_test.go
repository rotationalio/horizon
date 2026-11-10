package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
)

func TestDecisions_Validate(t *testing.T) {
	threshold := func(v float64) *float64 { return &v }

	valid := []struct {
		name string
		spec *provider.Decisions
	}{
		{"Noul", &provider.Decisions{Questions: map[string]provider.Question{
			"a": {Type: provider.QuestionNoul, Instructions: "?"}}}},
		{"ChoiceAndScore", &provider.Decisions{Questions: map[string]provider.Question{
			"a": {Type: provider.QuestionChoice, Criteria: map[string]string{"x": "y"}},
			"b": {Type: provider.QuestionScore, Criteria: []string{"low", "high"}}}}},
		{"Fields", &provider.Decisions{Fields: map[string]provider.Field{
			"a": {Type: provider.FieldText},
			"b": {Type: provider.FieldList, Items: "text", NullThreshold: threshold(1)}}}},
	}
	for _, tc := range valid {
		t.Run("Valid/"+tc.name, func(t *testing.T) {
			require.NoError(t, tc.spec.Validate())
		})
	}

	invalid := []struct {
		name string
		spec *provider.Decisions
		want string
	}{
		{"Nil", nil, "questions or fields"},
		{"Empty", &provider.Decisions{}, "questions or fields"},
		{"QuestionNoType", &provider.Decisions{Questions: map[string]provider.Question{"a": {}}}, "questions.a.type"},
		{"QuestionUnknownType", &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: "box"}}}, "not a supported question type"},
		{"ChoiceNoCriteria", &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionChoice}}}, "questions.a.criteria"},
		{"ScoreNoCriteria", &provider.Decisions{Questions: map[string]provider.Question{"a": {Type: provider.QuestionScore}}}, "questions.a.criteria"},
		{"FieldNoType", &provider.Decisions{Fields: map[string]provider.Field{"a": {}}}, "fields.a.type"},
		{"FieldUnknownType", &provider.Decisions{Fields: map[string]provider.Field{"a": {Type: "estimate"}}}, "not a supported field type"},
		{"ListNoItems", &provider.Decisions{Fields: map[string]provider.Field{"a": {Type: provider.FieldList}}}, "fields.a.items"},
		{"ThresholdZero", &provider.Decisions{Fields: map[string]provider.Field{"a": {Type: provider.FieldText, NullThreshold: threshold(0)}}}, "null_threshold"},
		{"ThresholdTooHigh", &provider.Decisions{Fields: map[string]provider.Field{"a": {Type: provider.FieldText, NullThreshold: threshold(1.5)}}}, "null_threshold"},
	}
	for _, tc := range invalid {
		t.Run("Invalid/"+tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestDecisions_Clone(t *testing.T) {
	var nilSpec *provider.Decisions
	require.Nil(t, nilSpec.Clone())

	spec := &provider.Decisions{
		Questions:     map[string]provider.Question{"a": {Type: provider.QuestionNoul}},
		Fields:        map[string]provider.Field{"b": {Type: provider.FieldText}},
		ReferenceDate: "2026-11-09",
	}
	cloned := spec.Clone()
	require.Equal(t, spec, cloned)

	cloned.Questions["c"] = provider.Question{Type: provider.QuestionNoul}
	cloned.Fields["d"] = provider.Field{Type: provider.FieldText}
	require.Len(t, spec.Questions, 1)
	require.Len(t, spec.Fields, 1)

	// Request.Clone carries the spec along with independent maps.
	req := &provider.Request{Decisions: spec}
	reqClone := req.Clone()
	reqClone.Decisions.Questions["e"] = provider.Question{Type: provider.QuestionNoul}
	require.Len(t, req.Decisions.Questions, 1)
}

func TestAnswers_Clone(t *testing.T) {
	var nilAnswers provider.Answers
	require.Nil(t, nilAnswers.Clone())

	answers := provider.Answers{"a": {Type: "noul", Raw: json.RawMessage(`{"type":"noul","noul":0.5}`)}}
	cloned := answers.Clone()
	require.Equal(t, answers, cloned)

	cloned["a"].Raw[0] = 'X'
	cloned["b"] = provider.Answer{Type: "noul"}
	require.Equal(t, byte('{'), answers["a"].Raw[0], "raw payload must not be shared")
	require.Len(t, answers, 1)

	// Response.Clone carries answers along independently.
	rep := &provider.Response{Answers: answers}
	repClone := rep.Clone()
	repClone.Answers["a"].Raw[1] = 'X'
	require.Equal(t, byte('"'), rep.Answers["a"].Raw[1])
}
