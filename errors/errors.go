package errors

import (
	"errors"
	"fmt"
)

var (
	// Catalog-related errors
	ErrCatalogConnectivity    = errors.New("catalog connectivity check failed")
	ErrCatalogRequired        = errors.New("catalog client is required")
	ErrInvalidCatalogEndpoint = errors.New("invalid catalog endpoint")
	ErrModelSlugRequired      = errors.New("catalog model has no slug")
	ErrNoCatalogModels        = errors.New("catalog returned no models")

	// Inference-related errors
	ErrInferenceConnectivity    = errors.New("inference connectivity check failed")
	ErrInferenceRequired        = errors.New("inference client is required")
	ErrInvalidInferenceEndpoint = errors.New("invalid inference endpoint")
	ErrNoModelOutput            = errors.New("no output response returned from the model")

	// Capability tool-loop errors
	ErrCapabilityProviderRequired = errors.New("capability provider is required for tool calls")
	ErrToolCallInvalid            = errors.New("tool call arguments are invalid")
	ErrToolCallIDRequired         = errors.New("tool call ID is required")
	ErrToolNotAdvertised          = errors.New("tool call was not advertised")
	ErrMaxToolTurns               = errors.New("maximum tool turns exceeded")

	// HTTP-related errors
	ErrBaseEndpointRequired      = errors.New("base endpoint is required")
	ErrRequestFailed             = errors.New("HTTP request failed")
	ErrUnsupportedCredentialType = errors.New("unsupported credential type for HTTP requests")

	// Task-related errors
	ErrTaskArchiveBootstrapFailed = errors.New("horizon task archive bootstrap failed")
	ErrTaskRequired               = errors.New("task is required")
	ErrTaskModalitiesRequired     = errors.New("task modalities are required")

	// Schema-related errors
	ErrNoJSONSchema = errors.New("cannot derive JSON schema without schema data or URI")

	// Configuration errors
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrInvalidDefaultModel     = errors.New("invalid default model")
	ErrInvalidID               = errors.New("invalid ID")
	ErrUnsupportedAPIType      = errors.New("unsupported API type")
	ErrUnsupportedProviderType = errors.New("unsupported provider type")

	// General errors
	ErrNotImplemented = errors.New("not implemented")
)

// Reduce namespacing conflicts by adding error functions from the errors package.
var (
	New    = errors.New
	Fmt    = fmt.Errorf
	Is     = errors.Is
	As     = errors.As
	Join   = errors.Join
	Unwrap = errors.Unwrap
)
