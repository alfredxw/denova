package app

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

func TestAgentSessionIDCoversBuiltInModelAgents(t *testing.T) {
	for _, agentKind := range persistentAgentKinds() {
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
	for _, agentKind := range persistentAgentKinds() {
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

func TestConfigManagerScopedSessionsAreIsolated(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store}
	automationReq := ConfigManagerRequest{Origin: "automation", ResourceID: "daily-review"}
	loreReq := ConfigManagerRequest{Origin: "lore", ResourceID: "__config_manager_lore__"}

	automationID, err := configManagerSessionID(automationReq)
	if err != nil {
		t.Fatal(err)
	}
	loreID, err := configManagerSessionID(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if automationID == loreID {
		t.Fatalf("scoped config manager sessions should differ: %s", automationID)
	}
	automationSession, err := store.GetOrCreate(automationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := automationSession.Append(schema.UserMessage("自动化配置")); err != nil {
		t.Fatal(err)
	}
	loreSession, err := store.GetOrCreate(loreID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loreSession.Append(schema.UserMessage("资料库配置")); err != nil {
		t.Fatal(err)
	}

	automationHistory, err := app.ConfigManagerMessages(automationReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(automationHistory) != 1 || automationHistory[0].Content != "自动化配置" {
		t.Fatalf("automation history should stay scoped: %#v", automationHistory)
	}
	loreHistory, err := app.ConfigManagerMessages(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(loreHistory) != 1 || loreHistory[0].Content != "资料库配置" {
		t.Fatalf("lore history should stay scoped: %#v", loreHistory)
	}
	if err := app.ClearConfigManagerSession(automationReq); err != nil {
		t.Fatal(err)
	}
	loreHistory, err = app.ConfigManagerMessages(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(loreHistory) != 1 || loreHistory[0].Content != "资料库配置" {
		t.Fatalf("clearing automation should not clear lore history: %#v", loreHistory)
	}
}

func persistentAgentKinds() []string {
	var kinds []string
	for _, definition := range config.AgentKindDefinitions() {
		if definition.SessionID != "" {
			kinds = append(kinds, definition.Kind)
		}
	}
	return kinds
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
