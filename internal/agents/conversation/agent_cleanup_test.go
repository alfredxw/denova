package conversation

import (
	"context"
	"strings"
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentCleanupManagerRespectsDisabledCompactionPolicy(t *testing.T) {
	compactionEnabled := false
	toolResultContextEnabled := true
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{
		IDE: config.AgentContextOverride{
			CompactionEnabled:        &compactionEnabled,
			ToolResultContextEnabled: &toolResultContextEnabled,
		},
	}}
	manager := NewAgentCleanupManager(cfg, config.AgentKindIDE)
	if manager == nil {
		t.Fatal("cleanup manager should remain enabled")
	}
	window := config.ResolveAgentModel(cfg, config.AgentKindIDE).ContextWindowTokens
	messages := []*agent.Message{{Role: agent.User, Content: strings.Repeat("x", window*4)}}

	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages:            messages,
		ModelRequest:        messages,
		CompactionAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CleanupNone || plan.Reason != "compaction_disabled" || plan.FallbackToCompaction {
		t.Fatalf("disabled Compaction policy must not schedule Compaction: %#v", plan)
	}
	if plan.Metrics.BodyPressureAfter != plan.Metrics.BodyPressureBefore {
		t.Fatalf("no-op cleanup must preserve body pressure metrics: %#v", plan.Metrics)
	}
}

func TestAgentCleanupManagerUsesConcreteDefinitionModelWindow(t *testing.T) {
	const childWindow = 32_000
	manager := NewAgentCleanupManagerForModel(&config.Config{}, config.AgentKindIDE, childWindow)
	if manager == nil {
		t.Fatal("cleanup manager should be enabled for IDE policy")
	}
	messages := []*agent.Message{agent.UserMessage("small child request")}
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages: messages, ModelRequest: messages, CompactionAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metrics.ContextWindowTokens != childWindow {
		t.Fatalf("Cleanup context window = %d, want concrete child window %d", plan.Metrics.ContextWindowTokens, childWindow)
	}
	if identity := manager.Identity(); identity.Kind != "cleanup.standard" {
		t.Fatalf("Cleanup authority = %#v, want public Standard manager", identity)
	}
}
