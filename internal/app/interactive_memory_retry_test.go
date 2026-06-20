package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"nova/config"
	"nova/internal/interactive"
)

func TestRunInteractiveMemoryAgentWithRetryUsesPreviousError(t *testing.T) {
	attempts := 0
	var retryInstruction string
	generate := func(_ context.Context, _ *config.Config, instruction string) (string, error) {
		attempts++
		if attempts == 1 {
			return `{"story_memory_patches":[{"op":"append","structure_id":"plot_summary","values":{"sequence":`, nil
		}
		retryInstruction = instruction
		return `{"story_memory_patches":[{"op":"append","structure_id":"plot_summary","values":{"sequence":"1","event":"主角进入旧宅。"}}]}`, nil
	}

	result, err := runInteractiveMemoryAgentWithRetry(context.Background(), &config.Config{}, "基础指令", nil, generate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if !strings.Contains(retryInstruction, "上次输出失败") || !strings.Contains(retryInstruction, "values 的所有值必须是文本") {
		t.Fatalf("retry instruction missing repair feedback:\n%s", retryInstruction)
	}
	if got := result.StoryMemoryPatches[0].Values["sequence"]; got != "1" {
		t.Fatalf("unexpected retry result: %#v", result.StoryMemoryPatches)
	}
}

func TestRunInteractiveMemoryAgentWithRetryRetriesApplyFailure(t *testing.T) {
	attempts := 0
	applyAttempts := 0
	generate := func(_ context.Context, _ *config.Config, _ string) (string, error) {
		attempts++
		return `{"story_memory_patches":[{"op":"append","structure_id":"plot_summary","values":{"event":"主角进入旧宅。"}}]}`, nil
	}
	apply := func(interactiveMemoryAgentResult) error {
		applyAttempts++
		if applyAttempts == 1 {
			return errors.New("故事记忆内容不能为空")
		}
		return nil
	}

	if _, err := runInteractiveMemoryAgentWithRetry(context.Background(), &config.Config{}, "基础指令", nil, generate, apply); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || applyAttempts != 2 {
		t.Fatalf("expected retry after apply failure, generate=%d apply=%d", attempts, applyAttempts)
	}
}

func TestRunInteractiveMemoryAgentWithRetryStopsWhenContextDone(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	generate := func(_ context.Context, _ *config.Config, _ string) (string, error) {
		attempts++
		return "", context.DeadlineExceeded
	}

	_, err := runInteractiveMemoryAgentWithRetry(ctx, &config.Config{}, "基础指令", nil, generate, nil)
	if err == nil {
		t.Fatal("expected context error")
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt after context done, got %d", attempts)
	}
	if strings.Contains(err.Error(), "重试 3 次仍失败") {
		t.Fatalf("context timeout should not be reported as exhausted retries: %v", err)
	}
}

func TestRunStoryMemoryGenerateAutoMarksReadyWhenAgentFails(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "旧宅",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我推开旧宅大门",
		Narrative: "门轴发出刺耳声响，屋内一片漆黑。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.MemoryStatus != "pending" || turn.StateStatus != "pending" {
		t.Fatalf("expected pending turn before auto generate: %#v", turn)
	}

	previousGenerate := generateInteractiveStateForStoryMemory
	generateInteractiveStateForStoryMemory = func(context.Context, *config.Config, string) (string, error) {
		return "", context.DeadlineExceeded
	}
	t.Cleanup(func() {
		generateInteractiveStateForStoryMemory = previousGenerate
	})

	app := &App{
		cfg:         &config.Config{NovaDir: novaDir, Workspace: workspace},
		workspace:   workspace,
		interactive: store,
	}
	state, patchCount, err := app.interactiveService().runStoryMemoryGenerate(context.Background(), story.ID, "main", storyMemoryGenerateSourceAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if patchCount != 0 {
		t.Fatalf("auto fallback should not apply patches, got %d", patchCount)
	}
	if state.SyncStatus != "ready" || state.SyncError != "" {
		t.Fatalf("unexpected story memory sync state: %#v", state)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil {
		t.Fatal("missing current turn")
	}
	if snapshot.CurrentTurn.MemoryStatus != "ready" || snapshot.CurrentTurn.StateStatus != "ready" {
		t.Fatalf("auto fallback should release pending state, got memory=%q state=%q", snapshot.CurrentTurn.MemoryStatus, snapshot.CurrentTurn.StateStatus)
	}
	if snapshot.CurrentTurn.MemoryError != "" || snapshot.CurrentTurn.StateError != "" {
		t.Fatalf("auto fallback should not persist visible errors: %#v", snapshot.CurrentTurn)
	}
}
