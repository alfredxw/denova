package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/prompts"
	producttools "denova/internal/agents/tools"
)

func TestWritingAssemblyComposesSkillAndHarnessStateReadAdapters(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAgentSkillFixture(t, skillsDir, "continuity", "Check narrative continuity before revising scenes.")

	cfg := &config.Config{
		Workspace: workspace, SkillsDir: skillsDir, DenovaDir: filepath.Join(root, ".denova"),
	}
	cfg.SetDataDir(filepath.Join(root, "data"))
	manager, err := harnessstate.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	harnessAdapter, err := harnessstate.NewReadAdapter(manager)
	if err != nil {
		t.Fatal(err)
	}
	harnessBinding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, harnessAdapter)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.ResolvedAgentToolSettings{
		config.AgentToolFilesystemRead: true,
		config.AgentToolSkills:         true,
		config.AgentToolHarnessState:   true,
	}
	assembly, err := buildChatModelAgentAssembly(context.Background(), cfg, chatModelAgentAssemblySpec{
		Kind: config.AgentKindIDE, SystemPrompt: mustTestPromptComposition(t, config.AgentKindIDE, "Stable base instruction."),
		ToolSettings: settings, EnableSkills: true, ReadAdapters: []producttools.ReadAdapterBinding{harnessBinding},
	})
	if err != nil {
		t.Fatalf("compose Skill and Harness State read adapters: %v", err)
	}

	foundRead := false
	foundSkill := false
	for _, definition := range assembly.Tools {
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		foundRead = foundRead || info.Name == "read"
		foundSkill = foundSkill || info.Name == "skill"
	}
	if !foundRead || !foundSkill {
		t.Fatalf("combined assembly tools missing read or skill: read=%t skill=%t", foundRead, foundSkill)
	}
}

func TestWritingAndGameAssembliesInjectAvailableSkillDescriptions(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAgentSkillFixture(t, skillsDir, "continuity", "Check narrative continuity before revising scenes.")
	writeAgentSkillFixture(t, skillsDir, "dialogue", "Improve character-specific dialogue and subtext.")
	writeAgentSpecificSkillFixture(t, skillsDir, "writing-only", "Apply a writing-only revision workflow.", config.AgentKindIDE)
	writeAgentSpecificSkillFixture(t, skillsDir, "game-only", "Apply a game-only narrative workflow.", config.AgentKindInteractiveStory)

	cfg := &config.Config{Workspace: workspace, SkillsDir: skillsDir}
	cfg.SetDataDir(filepath.Join(root, "data"))
	settings := config.ResolvedAgentToolSettings{
		config.AgentToolSkills:         true,
		config.AgentToolFilesystemRead: true,
	}
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			base := mustTestPromptComposition(t, kind, "Stable base instruction.")
			assembly, err := buildChatModelAgentAssembly(context.Background(), cfg, chatModelAgentAssemblySpec{
				Kind: kind, SystemPrompt: base, ToolSettings: settings, EnableSkills: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			instruction := assembly.SystemPrompt.Instruction()
			for _, expected := range []string{
				"<skills_instructions>",
				"- continuity: Check narrative continuity before revising scenes.",
				"- dialogue: Improve character-specific dialogue and subtext.",
			} {
				if !strings.Contains(instruction, expected) {
					t.Fatalf("%s instruction missing %q:\n%s", kind, expected, instruction)
				}
			}
			modeExpected := map[string]string{
				config.AgentKindIDE:              "- writing-only: Apply a writing-only revision workflow.",
				config.AgentKindInteractiveStory: "- game-only: Apply a game-only narrative workflow.",
			}[kind]
			modeExcluded := map[string]string{
				config.AgentKindIDE:              "game-only",
				config.AgentKindInteractiveStory: "writing-only",
			}[kind]
			if !strings.Contains(instruction, modeExpected) || strings.Contains(instruction, modeExcluded) {
				t.Fatalf("%s catalog ignored Agent-specific visibility:\n%s", kind, instruction)
			}
			if strings.Index(instruction, "Stable base instruction.") >= strings.Index(instruction, "<skills_instructions>") {
				t.Fatalf("%s catalog did not preserve the stable system prefix", kind)
			}
			if assembly.SystemPrompt.Manifest()[len(assembly.SystemPrompt.Manifest())-1].ID != "available_skills" {
				t.Fatalf("%s catalog is not an attributable final system fragment: %#v", kind, assembly.SystemPrompt.Manifest())
			}

			var skillDescription string
			for _, definition := range assembly.Tools {
				info, infoErr := definition.Tool.Info(context.Background())
				if infoErr != nil {
					t.Fatal(infoErr)
				}
				if info.Name == "skill" {
					skillDescription = info.Desc
				}
			}
			if skillDescription == "" {
				t.Fatalf("%s assembly did not expose the Skill tool", kind)
			}
			if strings.Contains(skillDescription, "continuity") || strings.Contains(skillDescription, "dialogue") {
				t.Fatalf("%s Skill catalog leaked into provider tool schema: %q", kind, skillDescription)
			}
		})
	}
}

func TestPublicDefinitionReturnsTheInjectedSkillsComposition(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAgentSkillFixture(t, skillsDir, "structure", "Restructure a draft while preserving its intent.")

	tools := config.AgentToolOverride{
		config.AgentToolFilesystemRead: true, config.AgentToolSkills: true,
		config.AgentToolWorkspaceWrite: false, config.AgentToolShell: false,
		config.AgentToolWebSearch: false, config.AgentToolWebFetch: false,
		config.AgentToolBrowser: false, config.AgentToolAsk: false,
		config.AgentToolTodo: false, config.AgentToolGoal: false,
		config.AgentToolDelegation: false, config.AgentToolConfigRead: false,
		config.AgentToolConfigApply: false, config.AgentToolEventRead: false,
		config.AgentToolLoreRead: false, config.AgentToolLoreWrite: false,
		config.AgentToolImageGeneration: false,
	}
	cfg := &config.Config{
		Workspace: workspace, SkillsDir: skillsDir, DenovaDir: filepath.Join(root, ".denova"),
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "skills-catalog-test-model",
		AgentTools: config.AgentToolSettings{
			Default: tools,
			IDE:     config.AgentToolOverride{config.AgentToolImageGeneration: false},
		},
	}
	cfg.SetDataDir(filepath.Join(root, "data"))
	definition, composition, err := BuildDefinitionWithCompositionForHost(
		context.Background(), cfg, nil, prompts.IDEStoryTeller{}, AgentHostCapabilities{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Instructions != composition.Instruction() {
		t.Fatal("public Definition and composition receipt diverged after Skills injection")
	}
	if !strings.Contains(definition.Instructions, "- structure: Restructure a draft while preserving its intent.") {
		t.Fatalf("public Definition omitted the Skills catalog:\n%s", definition.Instructions)
	}
}

func writeAgentSkillFixture(t *testing.T, root, name, description string) {
	writeAgentSpecificSkillFixture(t, root, name, description, "")
}

func writeAgentSpecificSkillFixture(t *testing.T, root, name, description, agentKind string) {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n"
	if agentKind != "" {
		content += "agent: " + agentKind + "\n"
	}
	content += "---\n\n# Instructions\n\nFollow this Skill.\n"
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
