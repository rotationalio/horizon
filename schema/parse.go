package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.rtnl.ai/x/mime"
)

func NewParser(bestEffort bool) Parser {
	if bestEffort {
		return &BestEffortParser{}
	}
	return &SimpleParser{}
}

// A parser parses a text string into a schema type.
type Parser interface {
	Parse(text string, schema *Schema) (any, error)
}

// SimpleParser parses the text string into the schema types as-is.
type SimpleParser struct{}

func (p *SimpleParser) Parse(text string, schema *Schema) (any, error) {
	switch schema.MimeType {
	case mime.ApplicationJSON, mime.ApplicationSchemaJSON:
		return parseJSON(text)
	default:
		return text, nil
	}
}

// BestEffortParser tries to parse the text string into the schema type or returns an
// error if the text string cannot be parsed.
type BestEffortParser struct{}

func (p *BestEffortParser) Parse(text string, schema *Schema) (any, error) {
	text = strings.TrimSpace(text)

	switch schema.MimeType {
	case mime.ApplicationJSON, mime.ApplicationSchemaJSON:
		// For JSON parsing, trim leading and trailing content.
		if content, ok := findContent(text, "{", "}"); ok {
			text = content
		} else if content, ok := findContent(text, "[", "]"); ok {
			text = content
		} else if content, ok := findContent(text, "\"", "\""); ok {
			text = content
		}

		// TODO: Validate the JSON data against the schema.
		return parseJSON(text)
	default:
		// TODO: What handling is required for other mime types?
		return text, nil
	}
}

func parseJSON(text string) (raw json.RawMessage, err error) {
	if err = json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("could not unmarshal JSON data: %w", err)
	}
	return raw, nil
}

func findContent(text string, start, end string) (string, bool) {
	startIndex := strings.Index(text, start)
	if startIndex == -1 {
		return "", false
	}
	endIndex := strings.LastIndex(text, end)
	if endIndex == -1 {
		return "", false
	}
	return text[startIndex : endIndex+1], true
}
