package interactive

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
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

func TestBranchRuntimeConfigCommitSurvivesIndexProjectionFailure(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	story, err := store.CreateStory(CreateStoryRequest{Title: "Index projection failure"})
	if err != nil {
		t.Fatal(err)
	}
	claudeConfig := func(model string) conversationconfig.Config {
		return conversationconfig.Config{
			AgentKind:     config.AgentKindInteractiveStory,
			ProfileID:     "default",
			ThinkingLevel: "medium",
			ApprovalMode:  config.AgentApprovalAsk,
			Runtime: &config.RuntimeSelection{
				Kind:   config.RuntimeClaude,
				Claude: &config.ClaudeRuntimeSettings{Model: model},
			},
		}
	}
	initial, err := store.EnsureBranchRuntimeConfig(story.ID, "main", claudeConfig("claude-primary"))
	if err != nil {
		t.Fatal(err)
	}
	if initial.Revision != 1 {
		t.Fatalf("unexpected initial revision: %d", initial.Revision)
	}
	saved, err := store.SetBranchRuntimeConfig(story.ID, "main", claudeConfig("claude-alternate"), initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("unexpected second revision: %d", saved.Revision)
	}

	// Mimic macOS-style catalog failure: index.json stays readable while its
	// atomic replacement fails. A committed config switch must still succeed.
	indexPath := store.indexPath()
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	store.writeIndex = func(Index) error {
		return errors.New("index replacement failed")
	}

	// Revision 2 -> 3 with the model switched while the catalog cannot be replaced.
	switched, err := store.SetBranchRuntimeConfig(story.ID, "main", claudeConfig("claude-third"), saved.Revision)
	if err != nil {
		t.Fatalf("committed switch was reported as failed: %v", err)
	}
	if switched.Revision != 3 || switched.Config.Runtime.Claude.Model != "claude-third" {
		t.Fatalf("unexpected committed snapshot: %+v", switched)
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("index.json changed while its replacement was failing")
	}
	// The failed projection is kept as a consumable diagnostic and cleared on
	// consumption so a later PATCH can surface it without duplication.
	if got := store.ConsumeStoryProjectionFailure(story.ID); got == nil || !strings.Contains(got.Error(), "index replacement failed") {
		t.Fatalf("projection failure was not recorded: %v", got)
	}
	if got := store.ConsumeStoryProjectionFailure(story.ID); got != nil {
		t.Fatalf("projection failure was not cleared after consumption: %v", got)
	}

	// The canonical config lives in the Story journal: a cold reload sees the
	// committed switch regardless of the stale catalog.
	cold := NewStore(root)
	t.Cleanup(func() { _ = cold.Close() })
	reloaded, found, err := cold.BranchRuntimeConfig(story.ID, "main")
	if err != nil || !found || reloaded.Revision != 3 || reloaded.Config.Runtime.Claude.Model != "claude-third" {
		t.Fatalf("cold reload mismatch: %+v found=%t err=%v", reloaded, found, err)
	}

	// CAS protection is unchanged: retrying with the pre-switch revision still
	// conflicts, so the client simply keeps the authoritative revision returned
	// by the successful switch.
	if _, err := store.SetBranchRuntimeConfig(story.ID, "main", claudeConfig("claude-third"), saved.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict for stale retry, got %v", err)
	}

	// Committing an already-idle paused controller state in the same transaction
	// as the next switch must also survive the catalog failure.
	key, err := agentsession.NormalizeKey(agentsession.Key{
		Namespace:  "denova.game.story",
		ID:         story.ID,
		Attributes: map[string]string{"branch": "main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const capabilityName = "denova.external.control.v1"
	committed, err := store.SetBranchRuntimeConfigWithCapabilities(
		story.ID, "main", claudeConfig("claude-fourth"), switched.Revision, key,
		func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{capabilityName: json.RawMessage(`{"phase":"idle"}`)}, nil
		},
	)
	if err != nil {
		t.Fatalf("capability commit failed under index failure: %v", err)
	}
	if committed.Revision != 4 {
		t.Fatalf("unexpected revision after capability commit: %d", committed.Revision)
	}
	idleCapability, capFound, err := cold.LoadCapability(t.Context(), story.ID, key, capabilityName)
	if err != nil || !capFound || string(idleCapability) != `{"phase":"idle"}` {
		t.Fatalf("idle capability not durable: %s found=%t err=%v", idleCapability, capFound, err)
	}

	// The catalog is rebuildable: once injection is cleared the next commit
	// refreshes index.json again and the story row reappears.
	store.writeIndex = nil
	healed, err := store.SetBranchRuntimeConfig(story.ID, "main", claudeConfig("claude-fifth"), committed.Revision)
	if err != nil {
		t.Fatalf("commit after clearing injection failed: %v", err)
	}
	if healed.Revision != 5 {
		t.Fatalf("unexpected revision after healing: %d", healed.Revision)
	}
	indexHealed, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(indexAfter, indexHealed) {
		t.Fatal("index.json was not refreshed after the injection was cleared")
	}
	var healedCatalog Index
	if err := json.Unmarshal(indexHealed, &healedCatalog); err != nil {
		t.Fatal(err)
	}
	storyIndexed := false
	for _, summary := range healedCatalog.Stories {
		if summary.ID == story.ID {
			storyIndexed = true
			break
		}
	}
	if !storyIndexed {
		t.Fatal("story missing from refreshed catalog")
	}
	// A subsequent successful projection clears the stale diagnostic.
	if got := store.ConsumeStoryProjectionFailure(story.ID); got != nil {
		t.Fatalf("successful projection did not clear the recorded failure: %v", got)
	}

	// The initialization path (EnsureBranchRuntimeConfig) follows the same
	// log-only semantics: a seeded branch commits at revision 1 even while the
	// catalog replacement fails.
	seededStore := NewStore(root)
	t.Cleanup(func() { _ = seededStore.Close() })
	seededStory, err := seededStore.CreateStory(CreateStoryRequest{Title: "Ensure seed index failure"})
	if err != nil {
		t.Fatal(err)
	}
	seededStore.writeIndex = func(Index) error {
		return errors.New("index replacement failed")
	}
	seeded, err := seededStore.EnsureBranchRuntimeConfig(seededStory.ID, "main", claudeConfig("claude-primary"))
	if err != nil {
		t.Fatalf("initialization reported failed despite durable commit: %v", err)
	}
	if seeded.Revision != 1 {
		t.Fatalf("unexpected seeded revision: %d", seeded.Revision)
	}
	seededReload, found, err := seededStore.BranchRuntimeConfig(seededStory.ID, "main")
	if err != nil || !found || seededReload.Revision != 1 || seededReload.Config.Runtime.Claude.Model != "claude-primary" {
		t.Fatalf("seeded reload mismatch: %+v found=%t err=%v", seededReload, found, err)
	}
	if got := seededStore.ConsumeStoryProjectionFailure(seededStory.ID); got == nil {
		t.Fatal("seeded initialization did not record the projection failure")
	}
}
