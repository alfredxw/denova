package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	"denova/internal/interactive"
)

func TestApplyWritingSkillRuntimePolicyKeepsAvailableDefault(t *testing.T) {
	skillsDir := t.TempDir()
	writeIDEWritingSkill(t, skillsDir, "novel-standard")
	runtime := &ideChatRuntime{cfg: config.Config{
		SkillsDir:           skillsDir,
		WritingSkillDefault: "novel-standard",
		SubAgents: []config.SubAgentConfig{{
			ID:           "researcher",
			Description:  "Reads context.",
			SystemPrompt: "Return notes.",
		}},
	}}
	req := &agents.ChatRequest{Message: "帮我分析一下 progress.md 有没有问题"}

	if err := applyWritingSkillRuntimePolicy(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != "novel-standard" {
		t.Fatalf("writing skill = %s, want novel-standard", req.WritingSkill)
	}
	if len(runtime.cfg.SubAgents) != 1 || runtime.cfg.SubAgents[0].ID != "researcher" {
		t.Fatalf("writing skill selection should not mutate subagents: %+v", runtime.cfg.SubAgents)
	}
}

func TestApplyWritingSkillRuntimePolicyKeepsCustomSkillAsDynamicHintOnly(t *testing.T) {
	skillsDir := t.TempDir()
	writeIDEWritingSkill(t, skillsDir, "slow-burn")
	runtime := &ideChatRuntime{cfg: config.Config{SkillsDir: skillsDir, WritingSkillDefault: "novel-standard"}}
	req := &agents.ChatRequest{Message: "写一个雨夜重逢的场景", WritingSkill: "slow-burn"}

	if err := applyWritingSkillRuntimePolicy(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != "slow-burn" {
		t.Fatalf("writing skill = %s, want slow-burn", req.WritingSkill)
	}
	if runtime.cfg.GeneralSubAgents.IDE != nil || len(runtime.cfg.SubAgents) != 0 {
		t.Fatalf("writing skill selection should not mutate agent config: %+v", runtime.cfg)
	}
}

func TestApplyWritingSkillRuntimePolicyFallsBackFromUnavailablePreset(t *testing.T) {
	skillsDir := t.TempDir()
	writeIDEWritingSkill(t, skillsDir, config.DefaultWritingSkillName)
	runtime := &ideChatRuntime{cfg: config.Config{SkillsDir: skillsDir, WritingSkillDefault: "retired-preset"}}
	req := &agents.ChatRequest{Message: "继续写作"}

	if err := applyWritingSkillRuntimePolicy(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != config.DefaultWritingSkillName {
		t.Fatalf("writing skill = %s, want fallback %s", req.WritingSkill, config.DefaultWritingSkillName)
	}
}

func TestApplyWritingSkillRuntimePolicyRejectsUnclassifiedIDEUtility(t *testing.T) {
	skillsDir := t.TempDir()
	writeIDEWritingSkill(t, skillsDir, config.DefaultWritingSkillName)
	writeIDESkill(t, skillsDir, "humanizer", "category: writing\n")
	runtime := &ideChatRuntime{cfg: config.Config{SkillsDir: skillsDir}}
	req := &agents.ChatRequest{Message: "润色这一段", WritingSkill: "humanizer"}

	if err := applyWritingSkillRuntimePolicy(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != config.DefaultWritingSkillName {
		t.Fatalf("writing utility should not become workflow: got %s want %s", req.WritingSkill, config.DefaultWritingSkillName)
	}
}

func writeIDEWritingSkill(t *testing.T, root, name string) {
	t.Helper()
	writeIDESkill(t, root, name, "category: writing\ncapabilities: [writing-workflow]\n")
}

func writeIDESkill(t *testing.T, root, name, metadata string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: writing flow\n" + metadata + "agent: ide\n---\n\n# Writing\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRootAgentStartFailureRollsBackDisplayTaskRegistration(t *testing.T) {
	tests := []struct {
		name  string
		start func(*App) (*Task, error)
	}{
		{
			name: "writing",
			start: func(application *App) (*Task, error) {
				return application.StartTaskWithError(context.Background(), agents.ChatRequest{CommandID: "closed-writing-start", Message: "write"})
			},
		},
		{
			name: "game",
			start: func(application *App) (*Task, error) {
				story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "acceptance", StoryTellerID: "classic"})
				if err != nil {
					return nil, err
				}
				return application.StartInteractiveTaskWithError(context.Background(), InteractiveAgentStartRequest{
					CommandID: "closed-game-start", StoryID: story.ID, BranchID: "main",
					Message: "open", Locale: "zh-CN",
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			application, err := New(context.Background(), &config.Config{
				OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(application.Close)
			if err := application.chatService.Close(context.Background()); err != nil {
				t.Fatal(err)
			}

			task, err := tt.start(application)
			if err == nil || task != nil {
				t.Fatalf("start against closed durable runtime = task=%v err=%v", task, err)
			}
			application.mu.RLock()
			activeWriting := application.activeTask
			activeGame := application.activeInteractiveRun
			registered := len(application.workspaceTasks)
			application.mu.RUnlock()
			if activeWriting != nil || activeGame != nil || registered != 0 {
				t.Fatalf("failed durable acceptance leaked task registration writing=%v game=%v registered=%d", activeWriting, activeGame, registered)
			}
		})
	}
}

func TestWritingInitialStartDeduplicatesBeforeAllocatingAnotherTask(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	request := agents.ChatRequest{CommandID: "writing-initial-same", Message: "write the opening"}
	first, err := application.StartTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := application.StartTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first || replayed.ID() != first.ID() {
		t.Fatalf("same command did not return original task: first=%p/%s replay=%p/%s", first, first.ID(), replayed, replayed.ID())
	}

	conflict := request
	conflict.Message = "write a different opening"
	if task, err := application.StartTaskWithError(context.Background(), conflict); !errors.Is(err, ErrAgentCommandConflict) || task != nil {
		t.Fatalf("different payload reuse = task=%v err=%v", task, err)
	}
}

func TestConfigAndAutomationStartFailureRollBackTaskRegistration(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	automationTask, err := application.CreateAutomation(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "acceptance", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	followUpRun := automation.RunRecord{
		ID: "follow-up-acceptance", TaskID: automationTask.ID, Scope: automation.ScopeWorkspace,
		Workspace: root, SessionID: "automation-follow-up-acceptance", Status: automation.RunStatusSuccess,
	}
	if _, err := application.automation().storeAllWorkspaces().AppendRun(automationTask.ID, followUpRun); err != nil {
		t.Fatal(err)
	}
	if err := application.chatService.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if task := application.StartConfigManagerTask(context.Background(), ConfigManagerRequest{Instruction: "inspect config"}); task != nil {
		t.Fatalf("config manager start against closed durable runtime returned task %s", task.ID())
	}
	if task, _, startErr := application.StartAutomationTaskWithEvidence(context.Background(), automationTask.ID, automation.TriggerManual, nil); startErr == nil || task != nil {
		t.Fatalf("automation start against closed durable runtime = task=%v err=%v", task, startErr)
	}
	if task, _, startErr := application.ContinueAutomationRun(context.Background(), followUpRun.ID, "follow-up-command", "continue"); startErr == nil || task != nil {
		t.Fatalf("automation follow-up against closed durable runtime = task=%v err=%v", task, startErr)
	}

	application.mu.RLock()
	registered := len(application.workspaceTasks)
	claims := len(application.activeAutomationClaims)
	activeTasks := len(application.activeAutomationTasks)
	activeRuns := len(application.activeAutomationRuns)
	application.mu.RUnlock()
	if registered != 0 || claims != 0 || activeTasks != 0 || activeRuns != 0 {
		t.Fatalf("failed durable acceptance leaked state registered=%d claims=%d tasks=%d runs=%d", registered, claims, activeTasks, activeRuns)
	}
}
