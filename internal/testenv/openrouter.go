package testenv

import (
	"fmt"
	"strings"
	"testing"
)

// Low-cost paid models that support text and function/tool calling. Using very
// low-cost models is the easiest way to avoid rate limiting and unstable free
// inference providers.
var OpenRouterTextModels = []string{
	"google/gemini-2.5-flash-lite",
	"google/gemini-2.5-flash",
	"openai/gpt-4.1-mini",
}

// Low-cost paid models that support multimodal input and function/tool
// calling. Using very low-cost models is the easiest way to avoid rate limiting
// and unstable free inference providers.
var OpenRouterMultimodalModels = []string{
	"google/gemini-2.5-flash-lite",
	"google/gemini-2.5-flash",
	"openai/gpt-4.1-mini",
}

// Runs fn against models in the supplied order. Rate-limit failures are retried
// with the next model; all other failures stop the test immediately. The test
// fails after every model has been exhausted.
func RunWithModels(t testing.TB, models []string, fn func(model string) error) string {
	t.Helper()
	if len(models) == 0 {
		t.Fatal("OpenRouter model pool is empty")
		return ""
	}

	var lastErr error
	for i, model := range models {
		if err := fn(model); err == nil {
			return model
		} else if !IsRateLimitError(err) {
			t.Fatalf("OpenRouter model %q failed: %v", model, err)
		} else {
			lastErr = err
			if i+1 < len(models) {
				t.Logf("OpenRouter model %q was rate limited; retrying with %q", model, models[i+1])
			}
		}
	}

	t.Fatalf("all OpenRouter models were rate limited; last error: %v", lastErr)
	return ""
}

// Reports whether err represents an HTTP 429/rate-limit response from
// OpenRouter.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "429") ||
		strings.Contains(message, "rate limit") ||
		strings.Contains(message, "rate-limit")
}

// Adds the model that produced err to the error context.
func ModelError(model string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("OpenRouter model %q: %w", model, err)
}
