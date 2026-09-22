package openai

import (
	"encoding/json"

	"go.rtnl.ai/horizon/capabilities"
)

// toolResultText converts provider-neutral tool result content into text for
// OpenAI tool result fields.
func toolResultText(result capabilities.ToolResult) string {
	var text string
	for _, content := range result.Content {
		// TODO: deal with content that is not text
		if content.Text == "" {
			continue
		}
		if text != "" {
			text += "\n"
		}
		text += content.Text
	}
	if text != "" {
		return text
	}

	// Fall back to structured content if text is not available.
	if structured, ok := result.Metadata["structuredContent"]; ok {
		if data, err := json.Marshal(structured); err == nil {
			return string(data)
		}
	}
	return ""
}

// toolSchema turns JSON schema data into a map.
func toolSchema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// toolParameters returns the tool schema or an empty object schema.
func toolParameters(raw json.RawMessage) (map[string]any, error) {
	schema, err := toolSchema(raw)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		return map[string]any{"type": "object"}, nil
	}
	return schema, nil
}
