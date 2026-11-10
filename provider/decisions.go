package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.rtnl.ai/x/validation"
)

// Decisions holds the typed questions or fields a model answers about the input.
// Decision models answer [Question]s and extraction models answer [Field]s; a
// request must carry whichever the model's route needs.
type Decisions struct {
	Questions     map[string]Question `json:"questions,omitempty"`      // Decision models (/api/alpha/decisions).
	Fields        map[string]Field    `json:"fields,omitempty"`         // Extraction models (/api/alpha/extractions).
	ReferenceDate string              `json:"reference_date,omitempty"` // YYYY-MM-DD; extraction models only.
}

// Question types for decision models. Box is image-based and not offered yet.
const (
	QuestionNoul   = "noul"   // Yes/no; answered with the probability of yes.
	QuestionChoice = "choice" // One of a set of named options.
	QuestionScore  = "score"  // A position on an ordered scale.
)

// Question is one typed question for a decision model.
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	// Depends on Type: noul takes an optional "true"/"false" map, choice a map of
	// option to description, and score an ordered []string (lowest first).
	Criteria any `json:"criteria,omitempty"`
}

// Field types for extraction models. Estimate and forecast are not modeled yet.
const (
	FieldText     = "text"
	FieldNumber   = "number"
	FieldDate     = "date"
	FieldTime     = "time"
	FieldDatetime = "datetime"
	FieldDuration = "duration"
	FieldList     = "list"
)

// Field is one typed field for an extraction model.
type Field struct {
	Type          string   `json:"type"`
	Instructions  string   `json:"instructions,omitempty"`
	NullThreshold *float64 `json:"null_threshold,omitempty"` // 0 < value <= 1; provider default is 0.5.
	Items         string   `json:"items,omitempty"`          // Required for list fields: text, number or date.
	Unit          string   `json:"unit,omitempty"`           // number and duration fields.
	Integer       *bool    `json:"integer,omitempty"`        // number and duration fields.
	Decimals      *int     `json:"decimals,omitempty"`       // number and duration fields; 0 to 6.
}

// ParseDecisions decodes a JSON spec, rejecting unknown fields. It does not
// validate the structure; call [Decisions.Validate] for that.
func ParseDecisions(s string) (*Decisions, error) {
	var d Decisions
	decoder := json.NewDecoder(strings.NewReader(s))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Validate checks the spec's structure, not which types a given model supports.
func (d *Decisions) Validate() (err error) {
	if d == nil || (len(d.Questions) == 0 && len(d.Fields) == 0) {
		return validation.Error(nil, validation.Missing("questions or fields"))
	}

	for _, name := range slices.Sorted(maps.Keys(d.Questions)) {
		field := fmt.Sprintf("questions.%s", name)
		q := d.Questions[name]
		err = checkType(err, field, "question", q.Type, QuestionNoul, QuestionChoice, QuestionScore)
		if (q.Type == QuestionChoice || q.Type == QuestionScore) && q.Criteria == nil {
			err = validation.Error(err, validation.Missing(field+".criteria"))
		}
	}

	for _, name := range slices.Sorted(maps.Keys(d.Fields)) {
		field := fmt.Sprintf("fields.%s", name)
		f := d.Fields[name]
		err = checkType(err, field, "field", f.Type, FieldText, FieldNumber, FieldDate, FieldTime, FieldDatetime, FieldDuration, FieldList)
		if f.Type == FieldList && f.Items == "" {
			err = validation.Error(err, validation.Missing(field+".items"))
		}
		if f.NullThreshold != nil && (*f.NullThreshold <= 0 || *f.NullThreshold > 1) {
			err = validation.Error(err, validation.Incorrect(field+".null_threshold", "must be greater than 0 and at most 1"))
		}
	}

	return err
}

// checkType adds an error to err if typ is empty or not one of known.
func checkType(err error, field, kind, typ string, known ...string) error {
	switch {
	case typ == "":
		return validation.Error(err, validation.Missing(field+".type"))
	case !slices.Contains(known, typ):
		return validation.Error(err, validation.Incorrect(field+".type", fmt.Sprintf("%q is not a supported %s type", typ, kind)))
	}
	return err
}

// Clone returns a copy with independent maps. Question criteria values are shared.
func (d *Decisions) Clone() *Decisions {
	if d == nil {
		return nil
	}

	cloned := *d
	cloned.Questions = maps.Clone(d.Questions)
	cloned.Fields = maps.Clone(d.Fields)
	return &cloned
}

// Answer is one typed answer. Type (noul, choice, text, ...) says how to read Raw,
// the full answer object.
type Answer struct {
	Type string
	Raw  json.RawMessage
}

// Answers maps each question or field name to its answer.
type Answers map[string]Answer

// Clone returns a copy with independent raw payloads.
func (a Answers) Clone() Answers {
	if a == nil {
		return nil
	}

	cloned := make(Answers, len(a))
	for name, answer := range a {
		answer.Raw = slices.Clone(answer.Raw)
		cloned[name] = answer
	}
	return cloned
}
