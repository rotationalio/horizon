package params

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Common parameters for the OpenAI API.
// TODO: handle audio parameters
// TODO: handle stop sequences
// TODO: handle web search options
// TODO: handle include strings
const (
	FrequencyPenalty     = "frequency_penalty"
	LogProbs             = "logprobs"
	MaxTokens            = "max_tokens"
	MaxCompletionTokens  = "max_completion_tokens"
	PresencePenalty      = "presence_penalty"
	Seed                 = "seed"
	Temperature          = "temperature"
	TopLogProbs          = "top_logprobs"
	TopP                 = "top_p"
	PromptCacheKey       = "prompt_cache_key"
	SafetyIdentifier     = "safety_identifier"
	LogitBias            = "logit_bias"
	PromptCacheRetention = "prompt_cache_retention"
	ReasoningEffort      = "reasoning_effort"
	ServiceTier          = "service_tier"
	Verbosity            = "verbosity"
	ToolChoice           = "tool_choice"
	MaxOutputTokens      = "max_output_tokens"
	MaxToolCalls         = "max_tool_calls"
	Include              = "include"
	Truncation           = "truncation"
	ReasoningSummary     = "reasoning_summary"
)

// Omitted parameters that are used directly by Endeavor or are purposefully not supported.
const (
	Messages           = "messages"
	Model              = "model"
	N                  = "n"
	Store              = "store"
	ParallelToolCalls  = "parallel_tool_calls"
	User               = "user"
	Metadata           = "metadata"
	Modalities         = "modalities"
	StreamOptions      = "stream_options"
	FunctionCall       = "function_call"
	Functions          = "functions"
	Prediction         = "prediction" // TODO: should we support this?
	ResponseFormat     = "response_format"
	Tools              = "tools"
	MaxToolTurns       = "max_tool_turns"
	WebSearchOptions   = "web_search_options"
	Background         = "background"
	Instructions       = "instructions"
	PreviousResponseID = "previous_response_id"
	ContextManagement  = "context_management"
	Conversation       = "conversation"
	Prompt             = "prompt"
	Input              = "input"
)

// Number between -2.0 and 2.0. Positive values penalize new tokens based on their
// existing frequency in the text so far, decreasing the model's likelihood to
// repeat the same line verbatim.
func (p *Params) FrequencyPenalty() (float64, bool) {
	return p.Float(FrequencyPenalty)
}

// Whether to return log probabilities of the output tokens or not. If true,
// returns the log probabilities of each output token returned in the `content` of
// `message`.
func (p *Params) LogProbs() (bool, bool) {
	return p.Bool(LogProbs)
}

// An upper bound for the number of tokens that can be generated for a completion,
// including visible output tokens and reasoning tokens. If not provided, the MaxTokens
// parameter is also looked up as a possible value.
func (p *Params) MaxCompletionTokens() (int64, bool) {
	if val, ok := p.Int(MaxCompletionTokens); ok {
		return val, true
	}

	if val, ok := p.Int(MaxTokens); ok {
		return val, true
	}

	return 0, false
}

// The maximum number of tokens that can be generated in the chat completion. This
// value can be used to control costs for text generated via API.
func (p *Params) MaxTokens() (int64, bool) {
	return p.Int(MaxTokens)
}

// Number between -2.0 and 2.0. Positive values penalize new tokens based on
// whether they appear in the text so far, increasing the model's likelihood to
// talk about new topics.
func (p *Params) PresencePenalty() (float64, bool) {
	return p.Float(PresencePenalty)
}

// This feature is in Beta. If specified, our system will make a best effort to
// sample deterministically, such that repeated requests with the same `seed` and
// parameters should return the same result. Determinism is not guaranteed, and you
// should refer to the `system_fingerprint` response parameter to monitor changes
// in the backend.
func (p *Params) Seed() (int64, bool) {
	return p.Int(Seed)
}

// What sampling temperature to use, between 0 and 2. Higher values like 0.8 will
// make the output more random, while lower values like 0.2 will make it more
// focused and deterministic. We generally recommend altering this or `top_p` but
// not both.
func (p *Params) Temperature() (float64, bool) {
	return p.Float(Temperature)
}

// An integer between 0 and 20 specifying the number of most likely tokens to
// return at each token position, each with an associated log probability.
// `logprobs` must be set to `true` if this parameter is used.
func (p *Params) TopLogProbs() (int64, bool) {
	return p.Int(TopLogProbs)
}

// An alternative to sampling with temperature, called nucleus sampling, where the
// model considers the results of the tokens with top_p probability mass. So 0.1
// means only the tokens comprising the top 10% probability mass are considered.
//
// We generally recommend altering this or `temperature` but not both.
func (p *Params) TopP() (float64, bool) {
	return p.Float(TopP)
}

// Used by OpenAI to cache responses for similar requests to optimize your cache
// hit rates. Replaces the `user` field.
func (p *Params) PromptCacheKey() (string, bool) {
	return p.String(PromptCacheKey)
}

// A stable identifier used to help detect users of your application that may be
// violating OpenAI's usage policies. The IDs should be a string that uniquely
// identifies each user, with a maximum length of 64 characters. We recommend
// hashing their username or email address, in order to avoid sending us any
// identifying information.
func (p *Params) SafetyIdentifier() (string, bool) {
	return p.String(SafetyIdentifier)
}

// Modify the likelihood of specified tokens appearing in the completion.
//
// Accepts a JSON object that maps tokens (specified by their token ID in the
// tokenizer) to an associated bias value from -100 to 100. Mathematically, the
// bias is added to the logits generated by the model prior to sampling. The exact
// effect will vary per model, but values between -1 and 1 should decrease or
// increase likelihood of selection; values like -100 or 100 should result in a ban
// or exclusive selection of the relevant token.
func (p *Params) LogitBias() (out map[string]int64, ok bool) {
	var orig any
	if orig, ok = p.Get(LogitBias); !ok {
		return nil, false
	}

	switch v := orig.(type) {
	case map[string]int64:
		return v, true
	case map[string]any:
		var err error
		if out, err = convertLogitBias(v); err != nil {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

// Converts a map[string]any to a map[string]int64.
func convertLogitBias(v map[string]any) (out map[string]int64, err error) {
	out = make(map[string]int64, len(v))
	for k, v := range v {
		switch v := v.(type) {
		case int:
			out[k] = int64(v)
		case int64:
			out[k] = v
		case int8:
			out[k] = int64(v)
		case int16:
			out[k] = int64(v)
		case int32:
			out[k] = int64(v)
		case uint:
			out[k] = int64(v)
		case uint8:
			out[k] = int64(v)
		case uint16:
			out[k] = int64(v)
		case uint32:
			out[k] = int64(v)
		case uint64:
			out[k] = int64(v)
		case json.Number:
			var parsed int64
			if parsed, err = v.Int64(); err != nil {
				return nil, err
			}
			out[k] = parsed
		case string:
			var parsed int64
			if parsed, err = strconv.ParseInt(v, 10, 64); err != nil {
				return nil, err
			}
			out[k] = parsed
		default:
			return nil, fmt.Errorf("invalid logit bias value type: %T", v)
		}
	}
	return out, nil
}

// The retention policy for the prompt cache. Set to `24h` to enable extended
// prompt caching, which keeps cached prefixes active for longer, up to a maximum
// of 24 hours.
//
// Any of "in-memory", "24h".
func (p *Params) PromptCacheRetention() (string, bool) {
	return p.String(PromptCacheRetention)
}

// Constrains effort on reasoning for
// [reasoning models](https://platform.openai.com/docs/guides/reasoning). Currently
// supported values are `none`, `minimal`, `low`, `medium`, `high`, and `xhigh`.
// Reducing reasoning effort can result in faster responses and fewer tokens used
// on reasoning in a response.
//
//   - `gpt-5.1` defaults to `none`, which does not perform reasoning. The supported
//     reasoning values for `gpt-5.1` are `none`, `low`, `medium`, and `high`. Tool
//     calls are supported for all reasoning values in gpt-5.1.
//   - All models before `gpt-5.1` default to `medium` reasoning effort, and do not
//     support `none`.
//   - The `gpt-5-pro` model defaults to (and only supports) `high` reasoning effort.
//   - `xhigh` is supported for all models after `gpt-5.1-codex-max`.
//
// Any of "none", "minimal", "low", "medium", "high", "xhigh".
func (p *Params) ReasoningEffort() (string, bool) {
	return p.String(ReasoningEffort)
}

// Specifies the processing type used for serving the request.
//
//   - If set to 'auto', then the request will be processed with the service tier
//     configured in the Project settings. Unless otherwise configured, the Project
//     will use 'default'.
//   - If set to 'default', then the request will be processed with the standard
//     pricing and performance for the selected model.
//   - If set to '[flex](https://platform.openai.com/docs/guides/flex-processing)' or
//     '[priority](https://openai.com/api-priority-processing/)', then the request
//     will be processed with the corresponding service tier.
//   - When not set, the default behavior is 'auto'.
//
// When the `service_tier` parameter is set, the response body will include the
// `service_tier` value based on the processing mode actually used to serve the
// request. This response value may be different from the value set in the
// parameter.
//
// Any of "auto", "default", "flex", "scale", "priority".
func (p *Params) ServiceTier() (string, bool) {
	return p.String(ServiceTier)
}

// Constrains the verbosity of the model's response. Lower values will result in
// more concise responses, while higher values will result in more verbose
// responses. Currently supported values are `low`, `medium`, and `high`.
//
// Any of "low", "medium", "high".
func (p *Params) Verbosity() (string, bool) {
	return p.String(Verbosity)
}

// Controls which (if any) tool is called by the model. `none` means the model will
// not call any tool and instead generates a message. `auto` means the model can
// pick between generating a message or calling one or more tools. `required` means
// the model must call one or more tools. Specifying a particular tool via
// `{"type": "function", "function": {"name": "my_function"}}` forces the model to
// call that tool.
//
// `none` is the default when no tools are present. `auto` is the default if tools
// are present.
func (p *Params) ToolChoice() (string, bool) {
	return p.String(ToolChoice)
}

// An upper bound for the number of tokens that can be generated for a response,
// including visible output tokens and reasoning tokens. If not provided, the MaxTokens
// parameter is also looked up as a possible value.
func (p *Params) MaxOutputTokens() (int64, bool) {
	if val, ok := p.Int(MaxOutputTokens); ok {
		return val, true
	}

	if val, ok := p.Int(MaxTokens); ok {
		return val, true
	}

	return 0, false
}

// The maximum number of total calls to built-in tools that can be processed in a
// response. This maximum number applies across all built-in tool calls, not per
// individual tool. Any further attempts to call a tool by the model will be
// ignored.
func (p *Params) MaxToolCalls() (int64, bool) {
	return p.Int(MaxToolCalls)
}

// The maximum number of tool turns for the horizon tool loop, rather than on each
// individual model request.
func (p *Params) MaxToolTurns() (int64, bool) {
	return p.Int(MaxToolTurns)
}

// The truncation strategy to use for the model response.
//
//   - `auto`: If the input to this Response exceeds the model's context window size,
//     the model will truncate the response to fit the context window by dropping
//     items from the beginning of the conversation.
//   - `disabled` (default): If the input size will exceed the context window size
//     for a model, the request will fail with a 400 error.
//
// Any of "auto", "disabled".
func (p *Params) Truncation() (string, bool) {
	return p.String(Truncation)
}

// A summary of the reasoning performed by the model. This can be useful for
// debugging and understanding the model's reasoning process. One of `auto`,
// `concise`, or `detailed`.
//
// `concise` is supported for `computer-use-preview` models and all reasoning
// models after `gpt-5`.
//
// Any of "auto", "concise", "detailed".
func (p *Params) ReasoningSummary() (string, bool) {
	return p.String(ReasoningSummary)
}
