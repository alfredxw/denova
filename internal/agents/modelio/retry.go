package modelio

import (
	"denova/config"
	agent "github.com/alfredxw/denova/agent"
)

// ModelExecutionPolicy maps the persisted product retry setting to the shared
// per-response budget. Callers add their tool, iteration and idle policies.
func ModelExecutionPolicy(cfg *config.Config) agent.ExecutionPolicy {
	retries := 5
	if cfg != nil && cfg.ModelMaxRetries >= 0 {
		retries = cfg.ModelMaxRetries
	}
	return agent.ExecutionPolicy{
		Retry:            &agent.RetryConfig{Decide: agent.TransientRetry},
		RetryIdentity:    agent.CapabilityIdentity{Kind: "denova.retry.transient", Version: 1},
		ModelMaxAttempts: retries + 1,
	}
}
