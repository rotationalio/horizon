package horizon

import (
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/prompts/templates"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/render"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"golang.org/x/oauth2"
)

// Configuration aliases

type Config = config.Config

// Task definitions, inputs, and outputs

type Task = task.Task
type TaskModel = task.Model
type Parameters = params.Params
type TaskInput = task.Input
type TaskOutput = task.Output
type TaskOutcome = task.Outcome

// Task Runner interfaces

type Runner = task.Runner
type CapabilityRunner = task.CapabilityRunner
type InputProcessor = task.InputProcessor
type ContextProcessor = task.ContextProcessor
type AttachmentProcessor = task.AttachmentProcessor
type Renderer = task.Renderer
type InputGuard = task.InputGuard
type OutputGuard = task.OutputGuard

// Provider configuration inference, and catalog types

type Provider = provider.Provider
type ProviderConfig = provider.Config
type ProviderRequest = provider.Request
type ProviderResponse = provider.Response
type ProviderType = provider.ProviderType
type ProviderAPIType = provider.APIType
type CatalogModel = catalog.Model

// Credentials and auth types and constructors

type ProviderCredentials = auth.Credentials

// NewNoneCredentials creates credentials for a provider that does not require
// authentication.
func NewNoneCredentials() *ProviderCredentials {
	return auth.NewNone()
}

// NewAPIKeyCredentials creates credentials that authenticate with an API key.
func NewAPIKeyCredentials(apiKey string) *ProviderCredentials {
	return auth.NewAPIKey(apiKey)
}

// NewTokenCredentials creates credentials that authenticate with a bearer
// token.
func NewTokenCredentials(token string) *ProviderCredentials {
	return auth.NewToken(token)
}

// NewBasicCredentials creates credentials for HTTP basic authentication.
func NewBasicCredentials(username, password string) *ProviderCredentials {
	return auth.NewBasic(username, password)
}

// NewOAuth2ClientCredentials creates credentials for OAuth2 client
// authentication.
func NewOAuth2ClientCredentials(clientID, clientSecret string) *ProviderCredentials {
	return auth.NewOAuth2Client(clientID, clientSecret)
}

// NewOAuth2TokenCredentials creates credentials from a complete OAuth2 token.
func NewOAuth2TokenCredentials(token *oauth2.Token) *ProviderCredentials {
	return auth.NewOAuth2Token(token)
}

// NewOpenAIOrganizationCredentials creates credentials for an OpenAI
// organization and project.
func NewOpenAIOrganizationCredentials(apiKey, organization, project string) *ProviderCredentials {
	return auth.NewOpenAIOrganization(apiKey, organization, project)
}

// Prompt types

type Prompt = prompts.Prompt
type Prompts = prompts.Prompts
type PromptContext = prompts.Context
type PromptRole = prompts.Role
type PromptType = prompts.Type

// Renderer and template types

type RendererType = render.Type
type PromptTemplate = templates.Template
type PromptTemplates = templates.Templates

// Attachment types

type Attachment = attachments.Attachment
type Attachments = attachments.Attachments

// Task schemas and modality

type Schema = schema.Schema
type TaskInputSchema = schema.Input
type TaskOutputSchema = schema.Output
type Modality = modality.Modality

// Capability types used by the optional tool-calling runner interface.

type CapabilityName = capabilities.Name
type CapabilityDefinition = capabilities.Definition
type CapabilityRequest = capabilities.Request
type CapabilityResponse = capabilities.Response
type CapabilityError = capabilities.Error

// Common enum values used in task and provider configuration.

const (
	ProviderTypeMock             = provider.ProviderTypeMock
	ProviderTypeOpenAI           = provider.ProviderTypeOpenAI
	ProviderTypeOpenAICompatible = provider.ProviderTypeOpenAICompatible
	ProviderTypeOpenRouter       = provider.ProviderTypeOpenRouter

	ProviderAPITypeMock                  = provider.APITypeMock
	ProviderAPITypeOpenAIResponses       = provider.APITypeOpenAIResponses
	ProviderAPITypeOpenAIChatCompletions = provider.APITypeOpenAIChatCompletions

	PromptRoleUser      = prompts.RoleUser
	PromptRoleSystem    = prompts.RoleSystem
	PromptRoleDeveloper = prompts.RoleDeveloper
	PromptRoleAssistant = prompts.RoleAssistant
	PromptRoleTool      = prompts.RoleTool

	PromptTypeMessage       = prompts.TypeMessage
	PromptTypeToolCall      = prompts.TypeToolCall
	PromptTypeRefusal       = prompts.TypeRefusal
	PromptTypeContentFilter = prompts.TypeContentFilter
	PromptTypeReasoning     = prompts.TypeReasoning

	RendererSprintf = render.Sprintf
	RendererGo      = render.Go

	ModalityText     = modality.Text
	ModalityImage    = modality.Image
	ModalityAudio    = modality.Audio
	ModalityVideo    = modality.Video
	ModalityDocument = modality.Document

	TaskOutcomeSucceeded = task.OutcomeSucceeded
	TaskOutcomeFailed    = task.OutcomeFailed
	TaskOutcomeCancelled = task.OutcomeCancelled
)
