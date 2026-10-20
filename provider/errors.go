package provider

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
)

// StatusFromError returns a provider status based on the error.
func StatusFromError(err error) Status {
	if apiErr, ok := errors.AsType[*openai.Error](err); ok && isAuthRequired(apiErr) {
		return StatusAuthRequired
	}

	return StatusError
}

// Return true if the error is an authentication required error.
func isAuthRequired(err error) bool {
	if err == nil {
		return false
	}

	if apiErr, ok := errors.AsType[*openai.Error](err); ok {
		return isHTTPAuthStatus(apiErr.StatusCode)
	}

	return errorChainContainsHTTPAuthStatus(err)
}

// errorChainContainsHTTPAuthStatus checks if the error chain contains an HTTP
// unauthorized or forbidden status code.
func errorChainContainsHTTPAuthStatus(err error) bool {
	unauthorized := fmt.Sprintf("%d %s", http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
	forbidden := fmt.Sprintf("%d %s", http.StatusForbidden, http.StatusText(http.StatusForbidden))

	var walk func(error) bool
	walk = func(e error) bool {
		if e == nil {
			return false
		}
		if apiErr, ok := errors.AsType[*openai.Error](e); ok {
			return isHTTPAuthStatus(apiErr.StatusCode)
		}
		msg := e.Error()
		if strings.Contains(msg, unauthorized) || strings.Contains(msg, forbidden) {
			return true
		}
		type unwrapperMulti interface{ Unwrap() []error }
		if mw, ok := e.(unwrapperMulti); ok {
			if slices.ContainsFunc(mw.Unwrap(), walk) {
				return true
			}
		}
		return walk(errors.Unwrap(e))
	}
	return walk(err)
}

// isHTTPAuthStatus reports whether code indicates missing or invalid credentials.
func isHTTPAuthStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}
