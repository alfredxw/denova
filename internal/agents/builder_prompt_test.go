package agents

import (
	"context"
	"crypto/sha256"
	"denova/internal/agents/run"
	"encoding/hex"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/prompts"
)

func TestAgentBuildFailsClosedBeforeModelConstructionOnPromptAdmission(t *testing.T) {
	cfg := builderPromptBudgetConfig(8, 1024, 16, 256)
	_, _, err := BuildWithComposition(context.Background(), cfg, nil, prompts.IDEStoryTeller{})
	if err == nil || !strings.Contains(err.Error(), "per-source limit") {
		t.Fatalf("Agent build should fail before model construction, got %v", err)
	}
	if _, composeErr := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindVersionSummary, "standalone", "", "builtin_base", "test", "test standalone admission", "123456789"); composeErr == nil {
		t.Fatal("standalone system prompt should use the same fail-closed admission")
	}
}

func TestBuildWithCompositionReturnsExactRunnerInstructionArtifact(t *testing.T) {
	var captured string
	previous := newNativeAgent
	newNativeAgent = func(_ context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
		if cfg.Name == "DenovaAgent" {
			captured = cfg.Instruction
		}
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newNativeAgent = previous })
	cfg := &config.Config{
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model",
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolWorkspaceRead: false, config.AgentToolWorkspaceWrite: false,
			config.AgentToolShell: false, config.AgentToolSkills: false,
			config.AgentToolLoreRead: false, config.AgentToolLoreWrite: false,
			config.AgentToolTodo: false, config.AgentToolWebSearch: false,
			config.AgentToolWebFetch: false, config.AgentToolDelegation: false,
		}},
	}
	_, composition, err := BuildWithComposition(context.Background(), cfg, nil, prompts.IDEStoryTeller{})
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{SystemPromptLog: composition}
	if captured != composition.Instruction() || options.SystemPromptLog.InstructionHash() != promptInstructionSHA(captured) {
		t.Fatalf("runner and agentrun.Options must share exact composition: runner_sha=%s option_sha=%s", promptInstructionSHA(captured), options.SystemPromptLog.InstructionHash())
	}
}

func promptInstructionSHA(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func builderPromptBudgetConfig(maxFragment, maxTotal, maxFragments, maxMetadata int) *config.Config {
	return &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		MaxFragmentBytes: &maxFragment, MaxTotalInjectedBytes: &maxTotal,
		MaxFragments: &maxFragments, MaxMetadataFieldBytes: &maxMetadata,
	}, VersionSummary: config.AgentContextOverride{
		MaxFragmentBytes: &maxFragment, MaxTotalInjectedBytes: &maxTotal,
		MaxFragments: &maxFragments, MaxMetadataFieldBytes: &maxMetadata,
	}}}
}
