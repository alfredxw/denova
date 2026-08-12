package agents

import (
	"context"
	"crypto/sha256"
	"denova/internal/agents/run"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/prompts"
)

func TestAgentBuildFailsClosedBeforeModelConstructionOnPromptAdmission(t *testing.T) {
	cfg := builderPromptBudgetConfig(8, 1024, 16, 256)
	_, _, err := BuildDefinitionWithCompositionForHost(context.Background(), cfg, nil, prompts.IDEStoryTeller{}, AgentHostCapabilities{})
	if err == nil || !strings.Contains(err.Error(), "per-source limit") {
		t.Fatalf("Agent build should fail before model construction, got %v", err)
	}
	if _, composeErr := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindVersionSummary, "standalone", "", "builtin_base", "test", "test standalone admission", "123456789"); composeErr == nil {
		t.Fatal("standalone system prompt should use the same fail-closed admission")
	}
}

func TestHarnessPromptIsLiveContextWithoutChangingAuditedInstruction(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	cfg := &config.Config{
		DenovaDir: dataDir, OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model",
		Labs: config.ResolvedLabs{ContinualLearning: true},
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolWorkspaceRead: false, config.AgentToolWorkspaceWrite: false,
			config.AgentToolShell: false, config.AgentToolSkills: false,
			config.AgentToolLoreRead: false, config.AgentToolLoreWrite: false,
			config.AgentToolTodo: false, config.AgentToolWebSearch: false,
			config.AgentToolWebFetch: false, config.AgentToolDelegation: false,
		}},
	}
	manager, err := harnessstate.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	base, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: base.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/ide.md", Content: []byte("Prefer concrete edits.")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, firstComposition, err := BuildDefinitionWithCompositionForHost(context.Background(), cfg, nil, prompts.IDEStoryTeller{}, AgentHostCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Instructions != firstComposition.Instruction() {
		t.Fatal("SystemPromptLog composition differs from Definition.Instructions")
	}
	firstFragments, err := first.Context.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstFragments) == 0 || firstFragments[0].Content != "Prefer concrete edits." || firstFragments[0].Revision != "" {
		t.Fatalf("unexpected live Harness prompt fragment: %#v", firstFragments)
	}

	_, err = manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: updated.Snapshot.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/ide.md", Content: []byte("Prefer verified edits.")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, secondComposition, err := BuildDefinitionWithCompositionForHost(context.Background(), cfg, nil, prompts.IDEStoryTeller{}, AgentHostCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	secondFragments, err := second.Context.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Instructions != secondComposition.Instruction() || second.Instructions != first.Instructions {
		t.Fatal("live Harness edit changed the durable or audited root instruction")
	}
	if second.Context.Identity() != first.Context.Identity() || second.Tools.Identity() != first.Tools.Identity() {
		t.Fatal("live Harness edit changed durable Agent capability identity")
	}
	if len(secondFragments) == 0 || secondFragments[0].Content != "Prefer verified edits." {
		t.Fatalf("next Agent build did not observe the live Harness prompt: %#v", secondFragments)
	}
}

func TestBuildDefinitionReturnsExactAuditedInstructionArtifact(t *testing.T) {
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
	definition, composition, err := BuildDefinitionWithCompositionForHost(context.Background(), cfg, nil, prompts.IDEStoryTeller{}, AgentHostCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{SystemPromptLog: composition}
	if definition.Instructions != composition.Instruction() || options.SystemPromptLog.InstructionHash() != promptInstructionSHA(definition.Instructions) {
		t.Fatalf("Definition and agentrun.Options must share exact composition: definition_sha=%s option_sha=%s", promptInstructionSHA(definition.Instructions), options.SystemPromptLog.InstructionHash())
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
