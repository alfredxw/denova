package tools

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"

	"denova/config"
)

const (
	AgentToolFileRead         = config.AgentToolFileRead
	AgentToolFileWrite        = config.AgentToolFileWrite
	AgentToolShellExecute     = config.AgentToolShellExecute
	AgentToolSkills           = config.AgentToolSkills
	AgentToolLoreRead         = config.AgentToolLoreRead
	AgentToolLoreWrite        = config.AgentToolLoreWrite
	AgentToolTodo             = config.AgentToolTodo
	AgentToolWebSearch        = config.AgentToolWebSearch
	AgentToolImageGeneration  = config.AgentToolImageGeneration
	AgentToolAgentConfigRead  = config.AgentToolAgentConfigRead
	AgentToolAgentConfigWrite = config.AgentToolAgentConfigWrite
)

type Capability = config.AgentToolCapability

// Capabilities returns all configurable tool families in stable UI/runtime order.
func Capabilities() []Capability {
	return config.AgentToolCapabilities()
}

type Settings = config.ResolvedAgentToolSettings

// Allowed reports whether a resolved settings projection permits a capability.
func Allowed(settings Settings, source string) bool {
	return config.AgentToolAllowed(settings, source)
}

type MiddlewareFactory func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error)
type ToolsFactory func(Settings) ([]tool.BaseTool, error)

type MiddlewareRegistration struct {
	Name    string
	Enabled func(Settings) bool
	Build   MiddlewareFactory
}

type ToolRegistration struct {
	Name    string
	Enabled func(Settings) bool
	Build   ToolsFactory
}

// BuildRequest contains the concrete adapters needed to assemble one Agent.
type BuildRequest struct {
	Settings    Settings
	Middlewares []MiddlewareRegistration
	Tools       []ToolRegistration
}

// BuildResult is the tool and middleware assembly consumed by an Agent builder.
type BuildResult struct {
	Tools    []tool.BaseTool
	Handlers []adk.ChatModelAgentMiddleware
}

// Build assembles model-callable tools and middleware in one stable order.
func Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	var result BuildResult
	for _, registration := range req.Middlewares {
		if registration.Build == nil || !enabled(registration.Enabled, req.Settings) {
			continue
		}
		mw, err := registration.Build(ctx, req.Settings)
		if err != nil {
			return BuildResult{}, err
		}
		if mw != nil {
			result.Handlers = append(result.Handlers, mw)
		}
	}
	for _, registration := range req.Tools {
		if registration.Build == nil || !enabled(registration.Enabled, req.Settings) {
			continue
		}
		tools, err := registration.Build(req.Settings)
		if err != nil {
			return BuildResult{}, err
		}
		result.Tools = append(result.Tools, tools...)
	}
	return result, nil
}

func enabled(fn func(Settings) bool, settings Settings) bool {
	return fn == nil || fn(settings)
}

func CapabilityAllowed(source string) func(Settings) bool {
	return func(settings Settings) bool {
		return Allowed(settings, source)
	}
}

func FilesystemAllowed(settings Settings) bool {
	return settings.FileRead || settings.FileWrite || settings.ShellExecute
}

func StaticTools(name string, tools ...tool.BaseTool) ToolRegistration {
	return ToolRegistration{
		Name: name,
		Build: func(Settings) ([]tool.BaseTool, error) {
			return append([]tool.BaseTool(nil), tools...), nil
		},
	}
}
