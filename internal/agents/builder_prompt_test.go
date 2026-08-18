package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentstate "github.com/alfredxw/denova/agent/state"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/prompts"
	"denova/internal/agents/run"
	"denova/internal/book"
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
		Labs: config.ResolvedLabs{DeveloperMode: true},
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolFilesystemRead: false, config.AgentToolWorkspaceWrite: false,
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
			config.AgentToolFilesystemRead: false, config.AgentToolWorkspaceWrite: false,
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

func TestProjectAgentsInjectInstructionFilesExactlyOnceAsLeadingContext(t *testing.T) {
	workspace := t.TempDir()
	agentInstructions := "Keep project changes focused and verify the result."
	creator := "Keep the narration restrained and concrete."
	if err := os.WriteFile(filepath.Join(workspace, book.AgentInstructionsFileName), []byte(agentInstructions), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, book.CreatorFileName), []byte(creator), 0o644); err != nil {
		t.Fatal(err)
	}
	state := book.NewState(workspace)
	cfg := &config.Config{
		Workspace: workspace, DenovaDir: filepath.Join(t.TempDir(), ".denova"),
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model",
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolFilesystemRead: false, config.AgentToolWorkspaceWrite: false,
			config.AgentToolShell: false, config.AgentToolSkills: false,
			config.AgentToolLoreRead: false, config.AgentToolLoreWrite: false,
			config.AgentToolTodo: false, config.AgentToolWebSearch: false,
			config.AgentToolWebFetch: false, config.AgentToolDelegation: false,
		}},
	}
	builders := map[string]func() (agent.Definition, error){
		"general": func() (agent.Definition, error) {
			definition, _, err := BuildGeneralDefinitionWithCompositionForHost(context.Background(), cfg, state, AgentHostCapabilities{})
			return definition, err
		},
		"writing": func() (agent.Definition, error) {
			definition, _, err := BuildDefinitionWithCompositionForHost(context.Background(), cfg, state, prompts.IDEStoryTeller{}, AgentHostCapabilities{})
			return definition, err
		},
		"game": func() (agent.Definition, error) {
			definition, _, err := BuildInteractiveStoryDefinitionWithCompositionForHost(context.Background(), cfg, state, prompts.InteractiveStorySystemInstructionInput{}, AgentHostCapabilities{})
			return definition, err
		},
		"director": func() (agent.Definition, error) {
			definition, _, err := BuildInteractiveDirectorDefinitionWithComposition(context.Background(), cfg, state)
			return definition, err
		},
		"config": func() (agent.Definition, error) {
			definition, _, err := BuildConfigManagerDefinitionWithCompositionForHost(context.Background(), cfg, state, AgentHostCapabilities{})
			return definition, err
		},
		"image": func() (agent.Definition, error) {
			definition, _, err := BuildImageDefinitionWithComposition(context.Background(), cfg, state, "")
			return definition, err
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			definition, err := build()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(definition.Instructions, agentInstructions) || strings.Contains(definition.Instructions, creator) {
				t.Fatalf("project instruction bodies must not be duplicated in the durable system prompt:\n%s", definition.Instructions)
			}
			fragments, err := definition.Context.Materialize(context.Background(), agent.ContextRequest{})
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{
				book.AgentInstructionsFileName: agentInstructions,
				book.CreatorFileName:           creator,
			}
			counts := make(map[string]int, len(expected))
			positions := make(map[string]int, len(expected))
			for index, fragment := range fragments {
				body, ok := expected[fragment.Resource]
				if !ok {
					continue
				}
				counts[fragment.Resource]++
				positions[fragment.Resource] = index
				if fragment.Placement != agent.ContextLeadingMessage || fragment.Role != agent.User || strings.Count(fragment.Content, body) != 1 || fragment.HardLimit <= 50*1024 {
					t.Fatalf("unexpected %s fragment: %#v", fragment.Resource, fragment)
				}
			}
			for resource := range expected {
				if counts[resource] != 1 {
					t.Fatalf("%s context fragment count = %d, want 1: %#v", resource, counts[resource], fragments)
				}
			}
			if positions[book.AgentInstructionsFileName] >= positions[book.CreatorFileName] {
				t.Fatalf("project instruction order is unstable: %#v", fragments)
			}

			owner, err := agent.New(context.Background(), definition, agent.WithSessionStore(agentsession.Memory()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			session, err := owner.Session(context.Background(), agent.NamedSession("project-instruction-inspection-"+name))
			if err != nil {
				t.Fatal(err)
			}
			inspection, err := session.Inspect(context.Background(), agent.Text("CURRENT_REQUEST"))
			if err != nil {
				t.Fatal(err)
			}
			var visible strings.Builder
			for _, message := range inspection.ModelRequest.Messages {
				if message != nil {
					visible.WriteString(message.Content)
					visible.WriteByte('\n')
				}
			}
			modelInput := visible.String()
			agentInstructionsIndex := strings.Index(modelInput, agentInstructions)
			creatorIndex := strings.Index(modelInput, creator)
			requestIndex := strings.Index(modelInput, "CURRENT_REQUEST")
			if strings.Count(modelInput, agentInstructions) != 1 || strings.Count(modelInput, creator) != 1 ||
				agentInstructionsIndex < 0 || creatorIndex <= agentInstructionsIndex || requestIndex <= creatorIndex {
				t.Fatalf("provider-visible project instruction placement is invalid: stable=%d\n%s", inspection.ModelRequest.StablePrefixMessages, modelInput)
			}
			if inspection.ModelRequest.StablePrefixMessages < 3 {
				t.Fatalf("system plus project instructions must form a stable leading prefix: %#v", inspection.ModelRequest)
			}
		})
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
