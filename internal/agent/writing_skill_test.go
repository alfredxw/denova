package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"

	"nova/config"
)

func TestResolveWritingSkillContextDefaultsToStandard(t *testing.T) {
	builtin := t.TempDir()
	writeTestSkill(t, builtin, config.DefaultWritingSkillName, "ide", "standard body")

	ctx, err := ResolveWritingSkillContext(context.Background(), &config.Config{
		SkillsDir: builtin,
		NovaDir:   t.TempDir(),
		Workspace: t.TempDir(),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Name != config.DefaultWritingSkillName {
		t.Fatalf("default writing skill = %s, want %s", ctx.Name, config.DefaultWritingSkillName)
	}
	if ctx.Scope != "builtin" || !strings.Contains(ctx.Content, "standard body") {
		t.Fatalf("unexpected default writing skill context: %#v", ctx)
	}
}

func TestResolveWritingSkillContextUsesWorkspaceUserBuiltinPriority(t *testing.T) {
	builtin := t.TempDir()
	novaDir := t.TempDir()
	workspace := t.TempDir()
	writeTestSkill(t, builtin, "novel-standard", "ide", "builtin body")
	writeTestSkill(t, filepath.Join(novaDir, "skills"), "novel-standard", "ide", "user body")
	writeTestSkill(t, filepath.Join(workspace, ".nova", "skills"), "novel-standard", "ide", "workspace body")

	ctx, err := ResolveWritingSkillContext(context.Background(), &config.Config{
		SkillsDir: builtin,
		NovaDir:   novaDir,
		Workspace: workspace,
	}, "novel-standard")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Scope != "workspace" {
		t.Fatalf("scope = %s, want workspace", ctx.Scope)
	}
	if !strings.Contains(ctx.Content, "workspace body") {
		t.Fatalf("workspace override content not selected: %#v", ctx)
	}
}

func TestArbitraryIDEWritingSkillCanBeSelectedAndInjected(t *testing.T) {
	novaDir := t.TempDir()
	writeTestSkill(t, filepath.Join(novaDir, "skills"), "slow-burn", "ide", "custom writing skill body")

	skillCtx, err := ResolveWritingSkillContext(context.Background(), &config.Config{
		NovaDir:   novaDir,
		Workspace: t.TempDir(),
	}, "slow-burn")
	if err != nil {
		t.Fatal(err)
	}

	composition := composeAgentInput(ChatRequest{
		Message:             "写一个暗流涌动的场景",
		WritingSkillContext: skillCtx,
	}, nil, nil, DefaultLoopPolicy())
	if !strings.Contains(composition.AgentMessage, "custom writing skill body") {
		t.Fatalf("selected skill content was not injected:\n%s", composition.AgentMessage)
	}
	if !strings.Contains(composition.AgentMessage, "不存在单独的 writing_scope 字段") {
		t.Fatalf("writing scope policy missing:\n%s", composition.AgentMessage)
	}
}

func TestNovelLiteDisablesGeneralAndConfiguredSubAgents(t *testing.T) {
	cfg := baseWritingPolicyConfig()
	cfg.SubAgents = []config.SubAgentConfig{{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Reads context",
		SystemPrompt: "Return notes.",
		Parents:      []string{config.AgentKindIDE},
	}}
	ApplyWritingSkillRolePolicy(cfg, "novel-lite")

	captured := captureBuiltDeepConfig(t, cfg)
	if !captured.WithoutGeneralSubAgent {
		t.Fatalf("novel-lite should disable the general subagent")
	}
	if len(captured.SubAgents) != 0 {
		t.Fatalf("novel-lite should disable configured subagents, got %d", len(captured.SubAgents))
	}
}

func TestNovelStandardExposesWriterReviewerFixer(t *testing.T) {
	cfg := baseWritingPolicyConfig()
	cfg.SubAgents = []config.SubAgentConfig{{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Reads context",
		SystemPrompt: "Return notes.",
		Parents:      []string{config.AgentKindIDE},
	}}
	ApplyWritingSkillRolePolicy(cfg, "novel-standard")

	captured := captureBuiltDeepConfig(t, cfg)
	if !captured.WithoutGeneralSubAgent {
		t.Fatalf("novel-standard should disable the general subagent")
	}
	if got := subAgentNames(captured.SubAgents); !reflect.DeepEqual(got, []string{"writer", "reviewer", "fixer"}) {
		t.Fatalf("standard subagents = %#v", got)
	}
}

func TestNovelHeavyExposesWritersRoomRoles(t *testing.T) {
	cfg := baseWritingPolicyConfig()
	ApplyWritingSkillRolePolicy(cfg, "novel-heavy")

	captured := captureBuiltDeepConfig(t, cfg)
	if !captured.WithoutGeneralSubAgent {
		t.Fatalf("novel-heavy should disable the general subagent")
	}
	want := []string{"context-planner", "writer", "reviewer", "fixer", "final-gate", "memory-patcher"}
	if got := subAgentNames(captured.SubAgents); !reflect.DeepEqual(got, want) {
		t.Fatalf("heavy subagents = %#v, want %#v", got, want)
	}
}

func TestUserSelectedWritingSkillDoesNotApplyPresetRolePolicy(t *testing.T) {
	cfg := baseWritingPolicyConfig()
	cfg.SubAgents = []config.SubAgentConfig{{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Reads context",
		SystemPrompt: "Return notes.",
		Parents:      []string{config.AgentKindIDE},
	}}
	ApplyWritingSkillRolePolicy(cfg, "slow-burn")

	captured := captureBuiltDeepConfig(t, cfg)
	if got := subAgentNames(captured.SubAgents); !reflect.DeepEqual(got, []string{"researcher"}) {
		t.Fatalf("custom skill should keep configured subagents, got %#v", got)
	}
}

func captureBuiltDeepConfig(t *testing.T, cfg *config.Config) *deep.Config {
	t.Helper()
	var captured *deep.Config
	previous := newDeepAgent
	newDeepAgent = func(_ context.Context, cfg *deep.Config) (adk.ResumableAgent, error) {
		copied := *cfg
		captured = &copied
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newDeepAgent = previous })

	_, err := buildDeepAgent(context.Background(), cfg, deepAgentSpec{
		Kind:        config.AgentKindIDE,
		Name:        "NovaAgent",
		Description: "test",
		Instruction: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured == nil {
		t.Fatalf("expected deep config to be captured")
	}
	return captured
}

func baseWritingPolicyConfig() *config.Config {
	off := false
	return &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				FileRead:     &off,
				FileWrite:    &off,
				ShellExecute: &off,
				Skills:       &off,
				LoreRead:     &off,
				LoreWrite:    &off,
				Todo:         &off,
				WebSearch:    &off,
			},
		},
	}
}

func subAgentNames(agents []adk.Agent) []string {
	names := make([]string, 0, len(agents))
	for _, sub := range agents {
		names = append(names, sub.Name(context.Background()))
	}
	return names
}

func writeTestSkill(t *testing.T, root, name, agentKind, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\nagent: " + agentKind + "\n---\n\n# " + name + "\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
