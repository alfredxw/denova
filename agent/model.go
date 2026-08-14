package agent

import (
	"context"
	"strings"
)

// ToolChoice controls whether a model may emit tool calls.
type ToolChoice string

const (
	ToolChoiceForbidden ToolChoice = "forbidden"
	ToolChoiceAllowed   ToolChoice = "allowed"
	ToolChoiceForced    ToolChoice = "forced"
)

// Options contains provider-neutral per-call model options.
type Options struct {
	Tools            []*ToolInfo
	MaxTokens        *int
	ToolChoice       *ToolChoice
	AllowedToolNames []string
	// SessionKey is the stable, provider-neutral cache-routing identity for
	// this conversation. Protocol adapters map it to provider wire fields.
	SessionKey string
}

// ModelOption immutably describes one per-call model setting.
type ModelOption struct {
	apply func(*Options)
}

// WithTools exposes the supplied tool schemas to a model call.
func WithTools(tools []*ToolInfo) ModelOption {
	copy := cloneToolInfos(tools)
	if tools == nil {
		copy = []*ToolInfo{}
	}
	return ModelOption{apply: func(options *Options) {
		options.Tools = cloneToolInfos(copy)
	}}
}

// WithMaxTokens sets a provider-neutral response token limit.
func WithMaxTokens(maxTokens int) ModelOption {
	return ModelOption{apply: func(options *Options) {
		value := maxTokens
		options.MaxTokens = &value
	}}
}

// WithToolChoice controls tool use and optionally restricts allowed names.
func WithToolChoice(choice ToolChoice, allowedToolNames ...string) ModelOption {
	allowed := append([]string(nil), allowedToolNames...)
	return ModelOption{apply: func(options *Options) {
		value := choice
		options.ToolChoice = &value
		options.AllowedToolNames = append([]string(nil), allowed...)
	}}
}

// WithSessionKey binds one stable conversation identity to a model call.
// Provider adapters decide whether it becomes a header or JSON body field.
func WithSessionKey(sessionKey string) ModelOption {
	sessionKey = strings.TrimSpace(sessionKey)
	return ModelOption{apply: func(options *Options) {
		options.SessionKey = sessionKey
	}}
}

// GetCommonOptions applies opts to a defensive copy of base.
func GetCommonOptions(base *Options, opts ...ModelOption) *Options {
	result := cloneModelOptions(base)
	for _, option := range opts {
		if option.apply != nil {
			option.apply(result)
		}
	}
	return result
}

// BindContextSessionKey returns detached call options that preserve an
// explicit SessionKey and otherwise inherit the caller's context value. Model
// adapters use this at their public Generate/Stream boundary so direct model
// calls and native Agent calls share the same behavior.
func BindContextSessionKey(ctx context.Context, base *Options, opts ...ModelOption) []ModelOption {
	result := append([]ModelOption(nil), opts...)
	if GetCommonOptions(base, opts...).SessionKey != "" {
		return result
	}
	if sessionKey, ok := SessionKeyFromContext(ctx); ok {
		result = append(result, WithSessionKey(sessionKey))
	}
	return result
}

func cloneModelOptions(options *Options) *Options {
	if options == nil {
		return &Options{}
	}
	clone := *options
	clone.Tools = cloneToolInfos(options.Tools)
	clone.AllowedToolNames = append([]string(nil), options.AllowedToolNames...)
	if options.MaxTokens != nil {
		value := *options.MaxTokens
		clone.MaxTokens = &value
	}
	if options.ToolChoice != nil {
		value := *options.ToolChoice
		clone.ToolChoice = &value
	}
	return &clone
}

// BaseChatModel is the complete provider-neutral model seam.
type BaseChatModel interface {
	Generate(ctx context.Context, input []*Message, opts ...ModelOption) (*Message, error)
	Stream(ctx context.Context, input []*Message, opts ...ModelOption) (*StreamReader[*Message], error)
}

// DefinitionModel is a declarative model whose stable identity is available
// after Definition initialization. Agent uses it to keep model construction
// and ModelIdentity inside the same agent.New composition boundary.
type DefinitionModel interface {
	BaseChatModel
	ModelIdentity() CapabilityIdentity
}

// ToolCallingChatModel can derive an immutable model with bound tools.
type ToolCallingChatModel interface {
	BaseChatModel
	WithTools(tools []*ToolInfo) (ToolCallingChatModel, error)
}
