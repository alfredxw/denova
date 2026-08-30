package session

import (
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
)

func testRuntimeConfig(agentKind string) conversationconfig.Config {
	return conversationconfig.Config{
		AgentKind: agentKind, ProfileID: "default", ThinkingLevel: "medium",
		ApprovalMode: config.AgentApprovalWrite,
	}
}

func TestRuntimeConfigPersistsWithCASAndMetadataProjection(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seed := testRuntimeConfig(config.AgentKindIDE)
	sess, err := store.CreateWithRuntimeConfig("Configured conversation", seed, ChannelAgent)
	if err != nil {
		t.Fatal(err)
	}
	initial, ok := sess.RuntimeConfig()
	if !ok || initial.Revision != 1 || initial.Config != seed {
		t.Fatalf("initial runtime config = %#v, present=%v", initial, ok)
	}

	next := seed
	next.ThinkingLevel = "high"
	next.ApprovalMode = config.AgentApprovalFullAccess
	saved, err := sess.SetRuntimeConfig(next, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 || saved.Config != next {
		t.Fatalf("saved runtime config = %#v", saved)
	}
	if _, err := sess.SetRuntimeConfig(seed, initial.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("stale writer should conflict, got %v", err)
	}
	switchedAgent := next
	switchedAgent.CustomAgentID = "focused-editor"
	if _, err := sess.SetRuntimeConfig(switchedAgent, saved.Revision); err == nil {
		t.Fatal("existing conversation must not switch custom Agent identity")
	}
	wrongKind := next
	wrongKind.AgentKind = config.AgentKindGeneral
	if _, err := sess.SetRuntimeConfig(wrongKind, saved.Revision); err == nil {
		t.Fatal("conversation Agent kind must be immutable")
	}
	if _, err := sess.EnsureRuntimeConfig(wrongKind); err == nil {
		t.Fatal("legacy initializer must not reinterpret an initialized conversation")
	}

	metas, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].RuntimeConfig == nil || *metas[0].RuntimeConfig != saved {
		t.Fatalf("metadata runtime config = %#v", metas)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	restoredSnapshot, ok := restored.RuntimeConfig()
	if !ok || restoredSnapshot != saved {
		t.Fatalf("restored runtime config = %#v, present=%v", restoredSnapshot, ok)
	}
}

func TestLegacyRuntimeConfigInitializationSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("Legacy conversation")
	if err != nil {
		t.Fatal(err)
	}
	seed := testRuntimeConfig(config.AgentKindIDE)
	initialized, err := sess.EnsureRuntimeConfig(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := restored.RuntimeConfig()
	if !ok || snapshot != initialized {
		t.Fatalf("restored legacy runtime config = %#v, present=%v; want %#v", snapshot, ok, initialized)
	}
}

func TestRuntimeConfigRefreshesAcrossOpenSessionHandles(t *testing.T) {
	dir := t.TempDir()
	firstStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seed := testRuntimeConfig(config.AgentKindIDE)
	first, err := firstStore.CreateWithRuntimeConfig("Shared conversation", seed, ChannelAgent)
	if err != nil {
		t.Fatal(err)
	}

	secondStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondStore.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := seed
	next.ThinkingLevel = "high"
	saved, err := first.SetRuntimeConfig(next, 1)
	if err != nil {
		t.Fatal(err)
	}

	// A canonical mutation forces the second handle to refresh the external
	// tail before it evaluates its compare-and-swap revision.
	final := next
	final.ApprovalMode = config.AgentApprovalAsk
	refreshed, err := second.SetRuntimeConfig(final, saved.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Revision != 3 || refreshed.Config != final {
		t.Fatalf("refreshed runtime config = %#v", refreshed)
	}
	firstSnapshot, ok := first.RuntimeConfig()
	if !ok || firstSnapshot != saved {
		t.Fatalf("first handle should remain an isolated snapshot until refresh: %#v, present=%v", firstSnapshot, ok)
	}
}
