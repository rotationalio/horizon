package provider

import (
	"errors"
	"fmt"

	openaigo "github.com/openai/openai-go/v3"
)

type providerError struct {
	message string
	err     error
}

func (e *providerError) Error() string { return e.message }
func (e *providerError) Unwrap() error { return e.err }

// Adds an HTTP status to provider errors when one is available. Non-HTTP
// errors are returned unchanged.
func newProviderError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openaigo.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode != 0 {
		return &providerError{
			message: fmt.Sprintf("%d: %s", apiErr.StatusCode, apiErr.Message),
			err:     err,
		}
	}

	return err
}
