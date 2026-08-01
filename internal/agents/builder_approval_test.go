package agents

import (
	"context"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
)

func TestToolApprovalModeIsSnapshottedWhenRunAssemblyIsBuilt(t *testing.T) {
	cfg := &config.Config{
		Workspace:            t.TempDir(),
		AgentApprovalMode:    config.AgentApprovalWrite,
		ShellEnvironmentMode: config.ShellEnvironmentProcess,
	}
	assembly, err := buildChatModelAgentAssembly(context.Background(), cfg, chatModelAgentAssemblySpec{
		Kind: agentrun.AgentKindIDE,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.AgentApprovalMode = config.AgentApprovalFullAccess
	for _, middleware := range assembly.Middlewares {
		orchestrator, ok := middleware.(*agenttoolruntime.OrchestratorMiddleware)
		if !ok {
			continue
		}
		if got := orchestrator.Configuration().ApprovalMode; got != config.AgentApprovalWrite {
			t.Fatalf("approval mode changed after assembly: %q", got)
		}
		return
	}
	t.Fatal("tool approval middleware was not installed")
}
