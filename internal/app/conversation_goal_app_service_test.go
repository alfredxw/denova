package app

import (
	"context"
	"testing"

	"denova/config"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

func TestConversationGoalSupportsWritingAgentChatAndInteractive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	application, err := New(ctx, &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	writing := ConversationConfigBinding{
		Mode: ConversationModeWriting, ProjectID: application.ProjectID(), SessionID: application.Session().ID,
	}
	created, err := application.MutateConversationGoal(ctx, writing, ConversationGoalMutation{
		Action: "set", Objective: "Complete the Writing goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != agent.GoalActive || created.Revision != 1 {
		t.Fatalf("created Writing goal = %#v", created)
	}
	loaded, found, err := application.ConversationGoal(ctx, writing)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.ID != created.ID {
		t.Fatalf("loaded Writing goal = %#v, found=%v", loaded, found)
	}

	agentChatSession, err := application.AgentChat().CreateSession(application.ProjectID(), "Goal test")
	if err != nil {
		t.Fatal(err)
	}
	agentChat := ConversationConfigBinding{
		Mode: ConversationModeAgentChat, ProjectID: application.ProjectID(), SessionID: agentChatSession.ID,
	}
	chatGoal, err := application.MutateConversationGoal(ctx, agentChat, ConversationGoalMutation{
		Action: "set", Objective: "Complete the Agent Chat goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if chatGoal.Status != agent.GoalActive || chatGoal.ID == created.ID {
		t.Fatalf("created Agent Chat goal = %#v", chatGoal)
	}

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Goal story", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	game := ConversationConfigBinding{
		Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main",
	}
	gameGoal, err := application.MutateConversationGoal(ctx, game, ConversationGoalMutation{Action: "set", Objective: "Complete the interactive story goal"})
	if err != nil {
		t.Fatal(err)
	}
	loadedGameGoal, found, err := application.ConversationGoal(ctx, game)
	if err != nil || !found || loadedGameGoal.ID != gameGoal.ID || loadedGameGoal.Status != agent.GoalActive {
		t.Fatalf("loaded interactive Goal=%#v found=%t error=%v", loadedGameGoal, found, err)
	}
}
