// Shared test environment helpers for Horizon.
package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joho/godotenv"
)

const defaultOpenRouterEndpoint = "https://openrouter.ai/api/v1"

// Loads repository test variables without overriding shell or CI variables.
func Load(t testing.TB) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Log("could not determine test file location; using process environment")
		return
	}

	envPath := filepath.Join(filepath.Dir(filename), "..", "..", ".env")
	values, err := godotenv.Read(envPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Logf("could not read .env at %s: %v", envPath, err)
		}
		return
	}

	for _, key := range []string{"OPENAI_API_KEY", "OPENROUTER_API_KEY", "OPENROUTER_ENDPOINT_URL"} {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if value, exists := values[key]; exists {
			_ = os.Setenv(key, value)
		}
	}
}

// Returns the configured OpenRouter API key.
func OpenRouterAPIKey(t testing.TB) string {
	t.Helper()
	Load(t)
	value := os.Getenv("OPENROUTER_API_KEY")
	if value == "" {
		t.Fatalf("OPENROUTER_API_KEY is not set")
	}
	return value
}

// Returns the configured OpenRouter endpoint or its default.
func OpenRouterEndpointURL(t testing.TB) string {
	t.Helper()
	Load(t)
	if value := os.Getenv("OPENROUTER_ENDPOINT_URL"); value != "" {
		return value
	}
	return defaultOpenRouterEndpoint
}

// Returns the configured OpenAI API key.
func OpenAIAPIKey(t testing.TB) string {
	t.Helper()
	Load(t)
	value := os.Getenv("OPENAI_API_KEY")
	if value == "" {
		t.Fatalf("OPENAI_API_KEY is not set")
	}
	return value
}

// Skips unstable provider tests unless explicitly enabled.
func RunUnstable(t testing.TB) {
	t.Helper()
	Load(t)
	if os.Getenv("HORIZON_RUN_UNSTABLE_TESTS") != "1" {
		t.Skip("unstable provider test disabled; set HORIZON_RUN_UNSTABLE_TESTS=1 to run")
	}
}
