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
	ErrInvalidModelOutput       = errors.New("model output does not match the requested format")
	ErrNoModelOutput            = errors.New("no output response returned from the model")

	// Capability tool-loop errors
	ErrCapabilityProviderRequired = errors.New("capability provider is required for tool calls")
	ErrMaxToolTurns               = errors.New("maximum tool turns exceeded")
	ErrToolCallIDRequired         = errors.New("tool call ID is required")
	ErrToolCallInvalid            = errors.New("tool call arguments are invalid")
	ErrToolNotAdvertised          = errors.New("tool call was not advertised")

	// HTTP-related errors
	ErrBaseEndpointRequired      = errors.New("base endpoint is required")
	ErrRequestFailed             = errors.New("HTTP request failed")
	ErrUnsupportedCredentialType = errors.New("unsupported credential type for HTTP requests")

	// Task-related errors
	ErrRunnerRequired             = errors.New("runner is required")
	ErrTaskArchiveBootstrapFailed = errors.New("horizon task archive bootstrap failed")
	ErrTaskExecutionFailed        = errors.New("task execution failed")
	ErrTaskModalitiesRequired     = errors.New("task modalities are required")
	ErrTaskOutputRequired         = errors.New("task output is required")
	ErrTaskRequired               = errors.New("task is required")

	// Prompt and message conversion errors
	ErrToolMessageMissingResults = errors.New("tool message must contain tool results")
	ErrUnsupportedMessageRole    = errors.New("unsupported message role")

	// Attachment errors
	ErrNilAttachment  = errors.New("attachment is nil")
	ErrInvalidDataURI = errors.New("invalid data URI")

	// Schema-related errors
	ErrInvalidJSONSchema               = errors.New("JSON schema must be an object")
	ErrNoJSONSchema                    = errors.New("cannot derive JSON schema without schema data or URI")
	ErrUnsupportedOutputSchemaMimeType = errors.New("unsupported output schema mime type")

	// Provider errors
	ErrProviderIDMismatch = errors.New("provider ID does not match config ID")
	ErrProviderIDRequired = errors.New("provider ID is required")
	ErrProviderNotFound   = errors.New("provider not found")
	ErrProviderRequired   = errors.New("provider is required")
	ErrUnsupportedAPIType = errors.New("unsupported API type")

	// OpenAI provider errors
	ErrNoChatCompletions                   = errors.New("no chat completions choices returned")
	ErrNoResponseOutput                    = errors.New("no response output items returned")
	ErrOpenAIResponsesError                = errors.New("openai responses error")
	ErrUnsupportedChatCompletionsAudioType = errors.New("unsupported chat completions audio content type")
	ErrUnsupportedChatToolCallType         = errors.New("unsupported chat completion tool call type")

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
