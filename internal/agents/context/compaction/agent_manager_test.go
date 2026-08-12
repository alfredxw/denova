package compaction

import (
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentManagerSummaryLimitUsesTightestTargetContextLimit(t *testing.T) {
	enabled := true
	fragmentBytes := 96 << 10
	totalBytes := 80 << 10
	providerBytes := 128 << 10
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		CompactionEnabled: &enabled,
		MaxFragmentBytes:  &fragmentBytes, MaxTotalInjectedBytes: &totalBytes,
		MaxProviderInputBytes: &providerBytes,
	}}}
	manager, err := NewAgentManager(cfg, config.AgentKindIDE, nil, agent.CapabilityIdentity{Kind: "test.model", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.SummaryLimitBytes(); got != totalBytes {
		t.Fatalf("summary limit = %d, want tightest target limit %d", got, totalBytes)
	}
}
