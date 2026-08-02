package agentruntime

import (
	"time"

	"denova/config"
)

// IdleTimeout resolves the optional per-Agent idle timeout. Zero means no
// timeout and is deliberately preserved for long-running model calls.
func IdleTimeout(cfg config.Config) time.Duration {
	if cfg.AgentIdleTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(cfg.AgentIdleTimeoutSeconds) * time.Second
}

// ToolResultMaxBytes resolves the hard context-admission limit for tool
// results. The configured default remains above 128 KiB.
func ToolResultMaxBytes(cfg config.Config) int {
	if cfg.AgentToolResultLimitKB <= 0 {
		return config.DefaultAgentToolResultLimitKB * 1024
	}
	return cfg.AgentToolResultLimitKB * 1024
}
