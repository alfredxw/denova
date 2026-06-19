package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nova/config"
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
