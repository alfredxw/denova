// Package scripttools assembles Denova's immediate and saved Script Tool
// definitions. JavaScript execution itself remains in the reusable agent module.
package scripttools

import (
	"context"
	"fmt"
	"time"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
	agentscript "github.com/alfredxw/denova/agent/script"
	publictools "github.com/alfredxw/denova/agent/tools"
)

// Saved materializes the current Harness contribution with the same Engine
// policy used by the immediate script tool.
func Saved(
	cfg *config.Config,
	harness harnessstate.Harness,
	agentKind string,
) ([]agent.ToolDefinition, error) {
	if !config.ResolveAgentTools(cfg, agentKind).Allows(config.AgentToolScript) {
		return nil, nil
	}
	scriptConfig, err := engineConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure Script Tools: %w", err)
	}
	targets := []string{agentKind}
	if definition, ok := config.FindActiveCustomAgent(cfg); ok && config.CustomAgentRuntimeKind(definition) == agentKind {
		targets = append(targets, definition.ID)
	}
	definitions, err := harness.ScriptToolDefinitions(targets, scriptConfig)
	if err != nil {
		return nil, fmt.Errorf("materialize Script Tools for Agent %s: %w", agentKind, err)
	}
	return definitions, nil
}

// Immediate constructs the model-visible script entry point.
func Immediate(cfg *config.Config) (agent.ToolDefinition, error) {
	scriptConfig, err := engineConfig(cfg)
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	definition, err := publictools.Script(scriptConfig)
	if err != nil {
		return agent.ToolDefinition{}, fmt.Errorf("build immediate script tool: %w", err)
	}
	return definition, nil
}

// ForSubAgent selects explicitly named saved tools from the parent's allowed
// definitions. The caller remains responsible for the parent capability ceiling.
func ForSubAgent(
	ctx context.Context,
	definitions []agent.ToolDefinition,
	enabled config.AgentToolOverride,
) ([]agent.ToolDefinition, error) {
	selected := make([]agent.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		info, err := definition.Tool.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info != nil && enabled[info.Name] {
			selected = append(selected, definition)
		}
	}
	return selected, nil
}

func engineConfig(cfg *config.Config) (publictools.ScriptConfig, error) {
	maxOutputBytes := config.DefaultAgentToolResultLimitKB * 1024
	var timeout time.Duration
	if cfg != nil {
		maxOutputBytes = toolresult.LimitBytes(cfg)
		if cfg.AgentScriptTimeoutSeconds > 0 {
			timeout = time.Duration(cfg.AgentScriptTimeoutSeconds) * time.Second
		}
	}
	engine, err := agentscript.NewEngine(agentscript.Config{
		MaxOutputBytes: maxOutputBytes,
	})
	if err != nil {
		return publictools.ScriptConfig{}, err
	}
	return publictools.ScriptConfig{Engine: engine, MaxResultBytes: maxOutputBytes, Timeout: timeout}, nil
}
