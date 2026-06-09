package app

import (
	"strings"
	"testing"

	"nova/config"
	"nova/internal/session"
)

func TestAgentSessionIDCoversBuiltInModelAgents(t *testing.T) {
	agents := []string{
		config.AgentKindLoreEditor,
		config.AgentKindTellerEditor,
		config.AgentKindInteractiveState,
		config.AgentKindInteractiveHotChoices,
		config.AgentKindVersionSummary,
	}

	for _, agentKind := range agents {
		id, ok := agentSessionID(agentKind)
		if !ok || id == "" {
			t.Fatalf("agent %s should have a persistent session id", agentKind)
		}
	}
}

func TestPersistAgentCallInStoreWritesFullMessages(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	longInput := strings.Repeat("输入", 7000)
	longOutput := strings.Repeat("输出", 5000)

	if err := persistAgentCallInStore(store, config.AgentKindInteractiveHotChoices, longInput, longOutput); err != nil {
		t.Fatal(err)
	}

	sess, err := agentSessionFromStore(store, config.AgentKindInteractiveHotChoices)
	if err != nil {
		t.Fatal(err)
	}
	history := sess.History()
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", history)
	}
	if history[0].Content != longInput || history[1].Content != longOutput {
		t.Fatalf("expected full persisted messages")
	}
	if sess.MessageCount() != 2 {
		t.Fatalf("message count = %d, want 2", sess.MessageCount())
	}
}

func TestClearAgentSessionInStoreMarksEffectiveContextForEveryBuiltInAgent(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := []string{
		config.AgentKindLoreEditor,
		config.AgentKindTellerEditor,
		config.AgentKindInteractiveState,
		config.AgentKindInteractiveHotChoices,
		config.AgentKindVersionSummary,
	}

	for _, agentKind := range agents {
		if err := persistAgentCallInStore(store, agentKind, "清理前", "旧输出"); err != nil {
			t.Fatalf("persist before clear %s: %v", agentKind, err)
		}
		if err := clearAgentSessionInStore(store, agentKind); err != nil {
			t.Fatalf("clear %s: %v", agentKind, err)
		}
		if err := persistAgentCallInStore(store, agentKind, "清理后", "新输出"); err != nil {
			t.Fatalf("persist after clear %s: %v", agentKind, err)
		}
		sess, err := agentSessionFromStore(store, agentKind)
		if err != nil {
			t.Fatal(err)
		}
		effective := sess.GetEffectiveMessages()
		if len(effective) != 2 || effective[0].Content != "清理后" || effective[1].Content != "新输出" {
			t.Fatalf("agent %s effective messages should only include messages after clear: %#v", agentKind, effective)
		}
		history := sess.History()
		hasClear := false
		for _, entry := range history {
			if entry.Type == "clear" {
				hasClear = true
				break
			}
		}
		if !hasClear {
			t.Fatalf("agent %s history should keep clear marker: %#v", agentKind, history)
		}
	}
}

func TestAppClearAgentSessionSupportsBackgroundAgents(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store}

	if err := app.ClearAgentSession(config.AgentKindVersionSummary); err != nil {
		t.Fatal(err)
	}
	history, err := app.AgentSessionMessages(config.AgentKindVersionSummary)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Type != "clear" {
		t.Fatalf("version summary agent should expose clear marker history: %#v", history)
	}
}
