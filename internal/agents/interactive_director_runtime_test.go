package agents

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	"errors"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/book"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestGenerateInteractiveDirectorWithToolsRequiresOwnedRuntime(t *testing.T) {
	workspace := t.TempDir()
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		nil,
		&config.Config{Workspace: workspace},
		book.NewState(workspace),
		agentinteractive.InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if err == nil || !strings.Contains(err.Error(), "运行时") {
		t.Fatalf("missing App-owned runtime error = %v", err)
	}
}

func TestGenerateInteractiveDirectorWithToolsRejectsMissingStoryState(t *testing.T) {
	service := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		service,
		&config.Config{},
		nil,
		agentinteractive.InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if err == nil || !strings.Contains(err.Error(), "故事状态") {
		t.Fatalf("missing story state error = %v", err)
	}
}

func TestGenerateInteractiveDirectorWithToolsRequiresCommandIDBeforeBuildingAgent(t *testing.T) {
	workspace := t.TempDir()
	service := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		service,
		&config.Config{Workspace: workspace},
		book.NewState(workspace),
		agentinteractive.InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if !errors.Is(err, runstate.ErrInvalidCommand) || !strings.Contains(err.Error(), "command_id") {
		t.Fatalf("missing Director command_id error = %v", err)
	}
}
