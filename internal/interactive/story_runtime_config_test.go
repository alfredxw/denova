package interactive

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/sessionjournal"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestSetBranchRuntimeConfigWithCapabilitiesIsAtomicSingleTransaction(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	story, err := store.CreateStory(CreateStoryRequest{Title: "Atomic config + capability"})
	if err != nil {
		t.Fatal(err)
	}
	initialCfg := conversationconfig.Config{
		AgentKind:     config.AgentKindInteractiveStory,
		ProfileID:     "default",
		ThinkingLevel: "medium",
		ApprovalMode:  config.AgentApprovalAsk,
		Runtime: &config.RuntimeSelection{
			Kind:   config.RuntimeClaude,
			Claude: &config.ClaudeRuntimeSettings{Model: "claude-v1"},
		},
	}
	saved, err := store.EnsureBranchRuntimeConfig(story.ID, "main", initialCfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := agentsession.NormalizeKey(agentsession.Key{
		Namespace:  "denova.game.story",
		ID:         story.ID,
		Attributes: map[string]string{"branch": "main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const capabilityName = "denova.external.control.v1"
	if err := store.UpdateCapabilities(t.Context(), story.ID, key, "test-init", func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
		return map[string]json.RawMessage{capabilityName: json.RawMessage(`{"phase":"suspended"}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	handleBefore := store.storyJournals[story.ID]
	cursorBefore := handleBefore.journal.Head().Cursor
	store.mu.Unlock()

	nextCfg := saved.Config
	nextCfg.Runtime = &config.RuntimeSelection{
		Kind:  config.RuntimeCodex,
		Codex: &config.CodexRuntimeSettings{Model: "codex-v1"},
	}

	// 1. Stale revision must not invoke capability update or append any record.
	updateCalled := false
	if _, err := store.SetBranchRuntimeConfigWithCapabilities(
		story.ID, "main", nextCfg, saved.Revision+1, key,
		func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
			updateCalled = true
			return map[string]json.RawMessage{capabilityName: json.RawMessage(`{"phase":"idle"}`)}, nil
		},
	); !errors.Is(err, conversationconfig.ErrRevisionConflict) || updateCalled {
		t.Fatalf("stale revision must fail before capability callback: err=%v called=%t", err, updateCalled)
	}

	// 2. Simulated physical journal write failure must leave both config and capability intact.
	domainLockPath := store.storyPath(story.ID) + ".domain.lock"
	_ = os.Remove(domainLockPath)
	if err := os.MkdirAll(domainLockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetBranchRuntimeConfigWithCapabilities(
		story.ID, "main", nextCfg, saved.Revision, key,
		func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{capabilityName: json.RawMessage(`{"phase":"idle"}`)}, nil
		},
	); err == nil {
		_ = os.Remove(domainLockPath)
		t.Fatal("expected write failure when domain lock path is blocked")
	}
	if err := os.Remove(domainLockPath); err != nil {
		t.Fatal(err)
	}

	coldAfterFail := NewStore(root)
	defer coldAfterFail.Close()
	cfgAfterFail, found, err := coldAfterFail.BranchRuntimeConfig(story.ID, "main")
	if err != nil || !found || cfgAfterFail.Revision != saved.Revision || cfgAfterFail.Engine().Kind != config.RuntimeClaude {
		t.Fatalf("write failure mutated config: %+v found=%t err=%v", cfgAfterFail, found, err)
	}
	capAfterFail, capFound, err := coldAfterFail.LoadCapability(t.Context(), story.ID, key, capabilityName)
	if err != nil || !capFound || string(capAfterFail) != `{"phase":"suspended"}` {
		t.Fatalf("write failure mutated capability: %s found=%t err=%v", capAfterFail, capFound, err)
	}

	// 3. Successful call must commit both config and capability in a single journal transaction.
	committed, err := store.SetBranchRuntimeConfigWithCapabilities(
		story.ID, "main", nextCfg, saved.Revision, key,
		func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{capabilityName: json.RawMessage(`{"phase":"idle"}`)}, nil
		},
	)
	if err != nil {
		t.Fatalf("atomic commit failed: %v", err)
	}
	store.mu.Lock()
	cursorAfter := store.storyJournals[story.ID].journal.Head().Cursor
	store.mu.Unlock()
	if cursorAfter != cursorBefore+1 {
		t.Fatalf("config and capability were not committed in one transaction: before=%d after=%d", cursorBefore, cursorAfter)
	}

	coldAfterSuccess := NewStore(root)
	defer coldAfterSuccess.Close()
	cfgAfterSuccess, found, err := coldAfterSuccess.BranchRuntimeConfig(story.ID, "main")
	if err != nil || !found || cfgAfterSuccess.Revision != committed.Revision || cfgAfterSuccess.Engine().Kind != config.RuntimeCodex {
		t.Fatalf("cold config reload mismatch: %+v found=%t err=%v", cfgAfterSuccess, found, err)
	}
	capAfterSuccess, capFound, err := coldAfterSuccess.LoadCapability(t.Context(), story.ID, key, capabilityName)
	if err != nil || !capFound || string(capAfterSuccess) != `{"phase":"idle"}` {
		t.Fatalf("cold capability reload mismatch: %s found=%t err=%v", capAfterSuccess, capFound, err)
	}
}
