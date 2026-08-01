package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestInteractiveImageRequiresBoundedCallerCommandIDBeforeWorkspaceAccess(t *testing.T) {
	service := &InteractiveAppService{app: &App{}}
	if _, err := service.GenerateInteractiveImage(context.Background(), "story", interactive.InteractiveImageGenerateRequest{}); !errors.Is(err, ErrAgentCommandIDRequired) {
		t.Fatalf("missing command_id error = %v", err)
	}
	request := interactive.InteractiveImageGenerateRequest{CommandID: strings.Repeat("x", 4097)}
	if _, err := service.GenerateInteractiveImage(context.Background(), "story", request); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized command_id error = %v", err)
	}
}

func TestInteractiveImageDisplayIdentityIsBoundToCommand(t *testing.T) {
	first := interactiveImageEventID("image-command-1")
	if first == "" || first != interactiveImageEventID("image-command-1") {
		t.Fatalf("same command produced unstable display identity: %q", first)
	}
	if first == interactiveImageEventID("image-command-2") {
		t.Fatalf("different commands produced the same display identity: %q", first)
	}
}

func TestImageAgentSemanticMessageIncludesBoundedContextHashes(t *testing.T) {
	base := ImageAgentGenerateRequest{
		CommandID: "image-command", Purpose: "interactive_image", StoryID: "story", BranchID: "main", TurnID: "turn",
		SourceContext: "secret scene one", SystemPrompt: "system one", ToolPrompt: "tool one",
	}
	first := imageAgentMessage(base)
	if strings.Contains(first, "secret scene one") {
		t.Fatalf("semantic message leaked raw source context: %s", first)
	}
	if first != imageAgentMessage(base) {
		t.Fatal("same image request produced an unstable semantic message")
	}
	changed := base
	changed.SourceContext = "secret scene two"
	if first == imageAgentMessage(changed) {
		t.Fatal("source context change was absent from image command semantics")
	}
}

func TestImageAgentMessageDeterministicallyLoadsRequestedSkill(t *testing.T) {
	req := ImageAgentGenerateRequest{SkillName: "interactive-image", Purpose: "interactive_image"}
	message := imageAgentMessage(req)
	if !strings.HasPrefix(message, "/interactive-image\n\n") || strings.Contains(message, "/<interactive-image>") {
		t.Fatalf("image Agent message uses a non-canonical Skill invocation: %q", message)
	}
	conversation := &imageAgentConversation{
		message: message,
		skillConfig: config.Config{
			SkillsDir: filepath.Join("..", "..", "skills"),
			DenovaDir: t.TempDir(),
		},
	}
	resolved, err := conversation.ResolveExplicitSkills(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Name != "interactive-image" || !strings.Contains(resolved[0].Instructions, "# 互动图像") {
		t.Fatalf("resolved image Skills = %#v", resolved)
	}
}

func TestShouldGenerateInteractiveImageModes(t *testing.T) {
	turns := []interactive.TurnEvent{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}}
	tests := []struct {
		name     string
		settings interactive.StoryImageSettings
		index    int
		source   string
		force    bool
		want     bool
		reason   string
	}{
		{name: "manual auto skip", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceAuto, want: false, reason: "manual_mode"},
		{name: "manual click generate", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceManual, want: true},
		{name: "one turn interval auto generate", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 1}, index: 0, source: interactiveImageSourceAuto, want: true},
		{name: "interval wait", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 3}, index: 1, source: interactiveImageSourceAuto, want: false, reason: "interval"},
		{name: "interval hit", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 3}, index: 2, source: interactiveImageSourceAuto, want: true},
		{name: "force ignores mode", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceAuto, force: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := shouldGenerateInteractiveImage(tt.settings, turns, tt.index, tt.source, tt.force)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("shouldGenerateInteractiveImage = (%v, %q), want (%v, %q)", got, reason, tt.want, tt.reason)
			}
		})
	}
}

func TestInteractiveImageSystemPromptUsesImagePreset(t *testing.T) {
	prompt := interactiveImageSystemPrompt(imagepreset.Preset{
		ID:   "realistic",
		Name: "写实",
		Slots: []imagepreset.Slot{
			{ID: "system", Name: "系统", Target: imagepreset.TargetAgentSystem, Enabled: true, Content: "理解真实光影。"},
			{ID: "tool", Name: "请求", Target: imagepreset.TargetToolRequest, Enabled: true, Content: "原样请求风格。"},
		},
	})
	if !strings.Contains(prompt, "图像方案预设") || !strings.Contains(prompt, "理解真实光影") {
		t.Fatalf("system prompt should include image preset:\n%s", prompt)
	}
	if strings.Contains(prompt, "原样请求风格") || strings.Contains(prompt, "image_prompt") || strings.Contains(prompt, "叙事编排") {
		t.Fatalf("system prompt should not mention legacy teller image_prompt:\n%s", prompt)
	}
}

func TestInteractiveImageSourceContextUsesBoundedTurnHistory(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "分支图像上下文", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID:  "main",
		User:      "进入密林",
		Narrative: "树影吞没了来路。",
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: first.ID, Title: "折返路线"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID:  branch.ID,
		User:      "折返回旧营地",
		Narrative: "主角在旧营地发现了一串新鲜脚印。",
	}); err != nil {
		t.Fatal(err)
	}
	storyCtx, err := store.StoryContext(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}

	context := interactiveImageSourceContext(storyCtx.Meta, storyCtx.Snapshot.Turns, 1)
	if !strings.Contains(context, "树影吞没了来路") || !strings.Contains(context, "新鲜脚印") {
		t.Fatalf("source context should use the current branch turn history:\n%s", context)
	}
}
