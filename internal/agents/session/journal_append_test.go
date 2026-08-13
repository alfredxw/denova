package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestColdReplayKeepsCanonicalMessagesAcrossLargeDisplayWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sparse-canonical.jsonl")
	lines := []string{`{"type":"session","id":"sparse-canonical","created_at":"2026-01-01T00:00:00Z"}`}
	appendJSON := func(value any) {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(encoded))
	}
	appendJSON(agent.UserMessage("canonical user input"))
	for index := 0; index < sessionRecentTransactionLimit+5; index++ {
		appendJSON(displayRecord{
			Type: historyTypeDisplay,
			DisplayEvent: DisplayEvent{
				ID: fmt.Sprintf("progress-%03d", index), Role: "thinking", Content: "display-only progress",
			},
		})
	}
	appendJSON(agent.AssistantMessage("canonical assistant output", nil))
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("sparse-canonical")
	if err != nil {
		t.Fatal(err)
	}
	messages := sess.GetEffectiveMessages()
	if len(messages) != 2 || messages[0].Role != agent.User || messages[0].Content != "canonical user input" ||
		messages[1].Role != agent.Assistant || messages[1].Content != "canonical assistant output" {
		t.Fatalf("cold canonical transcript = %#v", messages)
	}
}

func TestLegacyJournalAppendPreservesExistingBytesAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	legacy := []byte(strings.Join([]string{
		`{"type":"session","id":"legacy","created_at":"2026-01-01T00:00:00Z"}`,
		`{"role":"user","content":"旧问题"}`,
		"",
	}, "\n"))
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("新回答", nil)); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, legacy) {
		t.Fatalf("append must preserve every existing journal byte\nbefore=%s\nafter=%s", legacy, after)
	}
	if len(after) <= len(legacy) {
		t.Fatalf("journal did not grow: before=%d after=%d", len(legacy), len(after))
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	messages := reloaded.GetMessages()
	if len(messages) != 2 || messages[0].Content != "旧问题" || messages[1].Content != "新回答" {
		t.Fatalf("reloaded messages = %#v", messages)
	}
}

func TestDisplayUpdateAppendsPatchAndReloadsMaterializedState(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID: "call-1", Role: "tool_call", Name: "read", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "default.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := sess.UpdateDisplayToolResult("call-1", "read", "success", "章节内容"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Fatal("display update rewrote existing journal bytes")
	}
	if !bytes.Contains(after[len(before):], []byte(`"type":"display_patch"`)) {
		t.Fatalf("display update must append an explicit patch record: %s", after[len(before):])
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if len(history) != 1 || history[0].Status != "success" || history[0].Result != "章节内容" {
		t.Fatalf("display patch was not materialized after reload: %#v", history)
	}
}

func TestStreamedDisplayContentPersistsInLargeBatchesAndFlushesAtBoundary(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("display-stream-batch")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "assistant-stream", Role: "assistant"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "display-stream-batch.jsonl")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", displayStreamPersistBatchBytes-1)
	if err := sess.AppendDisplayEventContent("assistant-stream", "assistant", content); err != nil {
		t.Fatal(err)
	}
	buffered, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if buffered.Size() != before.Size() {
		t.Fatalf("sub-threshold stream changed journal size: before=%d after=%d", before.Size(), buffered.Size())
	}
	if err := sess.AppendDisplayEventContent("assistant-stream", "assistant", "y"); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Size() <= buffered.Size() {
		t.Fatal("display stream did not persist at the batch boundary")
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("display-stream-batch")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if len(history) != 1 {
		t.Fatalf("reloaded streamed display history length = %d, want 1", len(history))
	}
	if history[0].Content != content+"y" {
		t.Fatalf("reloaded streamed display content length = %d, want %d", len(history[0].Content), len(content)+1)
	}
}

func TestFinalizeDisplayAssistantRunPersistsPresentationPhases(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("display-phases")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []DisplayEvent{
		{ID: "assistant-progress", Role: "assistant", Content: "正在排查。", RunID: "run-phases", DisplayPhase: DisplayPhaseCandidate},
		{ID: "thinking-1", Role: "thinking", Content: "检查结果。", RunID: "run-phases"},
		{ID: "assistant-final", Role: "assistant", Content: "问题已修复。", RunID: "run-phases", DisplayPhase: DisplayPhaseCandidate},
	} {
		if err := sess.AppendDisplayEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.FinalizeDisplayAssistantRun("run-phases", "assistant-final", DisplayPhaseFinal); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("display-phases")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if len(history) != 3 || history[0].DisplayPhase != DisplayPhaseProgress || history[2].DisplayPhase != DisplayPhaseFinal {
		t.Fatalf("display phases were not restored from append-only patches: %#v", history)
	}
}

func TestLegacyDisplayRecordCanBePatchedAndReloaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	legacy := []byte(strings.Join([]string{
		`{"type":"session","id":"legacy","created_at":"2026-01-01T00:00:00Z"}`,
		`{"type":"display","id":"call-1","role":"tool_call","name":"read","status":"running"}`,
		"",
	}, "\n"))
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("call-1", "read", "success", "旧记录结果"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, legacy) {
		t.Fatal("patching a legacy display record rewrote old bytes")
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.History(); len(got) != 1 || got[0].Status != "success" || got[0].Result != "旧记录结果" {
		t.Fatalf("legacy display patch did not survive reload: %#v", got)
	}
}

func TestSessionAndInterruptionUpdatesUseExplicitPatches(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "default.jsonl")
	beforeRename, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Rename("新标题"); err != nil {
		t.Fatal(err)
	}
	afterRename, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterRename, beforeRename) || !bytes.Contains(afterRename[len(beforeRename):], []byte(`"type":"session_patch"`)) {
		t.Fatalf("rename did not append session_patch: %s", afterRename)
	}
	if err := sess.MarkInterrupted("继续写", "草稿", "stopped"); err != nil {
		t.Fatal(err)
	}
	pending := sess.PendingInterruption()
	if pending == nil {
		t.Fatal("expected pending interruption")
	}
	beforeResolve, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.ResolveInterruption(pending.ID); err != nil {
		t.Fatal(err)
	}
	afterResolve, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterResolve, beforeResolve) || !bytes.Contains(afterResolve[len(beforeResolve):], []byte(`"type":"interruption_patch"`)) {
		t.Fatalf("resolve did not append interruption_patch: %s", afterResolve[len(beforeResolve):])
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Title() != "新标题" || reloaded.PendingInterruption() != nil {
		t.Fatalf("patch projection after reload: title=%q pending=%#v", reloaded.Title(), reloaded.PendingInterruption())
	}
}

func TestLoaderIgnoresOnlyIncompleteTailAndNextAppendRepairsIt(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("完整消息")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "default.jsonl")
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const incomplete = `{"type":"display_patch"`
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(incomplete); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	recoveredStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := recoveredStore.Get("default")
	if err != nil {
		t.Fatalf("incomplete final record should be recoverable: %v", err)
	}
	if got := recovered.GetMessages(); len(got) != 1 || got[0].Content != "完整消息" {
		t.Fatalf("messages after tail recovery = %#v", got)
	}
	if err := recovered.Append(agent.AssistantMessage("恢复后消息", nil)); err != nil {
		t.Fatal(err)
	}

	repaired, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(repaired, valid) {
		t.Fatal("repair changed bytes before the incomplete tail")
	}
	if bytes.Contains(repaired, []byte(incomplete)) {
		t.Fatalf("incomplete tail remained before a later record: %s", repaired)
	}
	backups, err := filepath.Glob(path + ".incomplete-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("incomplete tail must be preserved once, backups=%v", backups)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != incomplete {
		t.Fatalf("incomplete tail backup = %q, want %q", backup, incomplete)
	}
	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetMessages(); len(got) != 2 || got[1].Content != "恢复后消息" {
		t.Fatalf("messages after repaired append = %#v", got)
	}
}

func TestLoaderAcceptsCompleteFinalRecordWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	content := strings.Join([]string{
		`{"type":"session","id":"legacy","created_at":"2026-01-01T00:00:00Z"}`,
		`{"role":"user","content":"无换行结尾"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("下一条", nil)); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetMessages(); len(got) != 2 || got[0].Content != "无换行结尾" || got[1].Content != "下一条" {
		t.Fatalf("messages after appending to no-LF journal = %#v", got)
	}
}

func TestLoaderRejectsUnknownCompleteFinalRecordWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jsonl")
	content := strings.Join([]string{
		`{"type":"session","id":"broken","created_at":"2026-01-01T00:00:00Z"}`,
		`{"type":"unknown_but_complete"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("broken"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("complete unknown final record must be rejected, got %v", err)
	}
}

func TestLoaderRejectsCorruptCompleteRecordInMiddle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jsonl")
	content := strings.Join([]string{
		`{"type":"session","id":"broken","created_at":"2026-01-01T00:00:00Z"}`,
		`{"role":"user","content":"第一条"}`,
		`{"type":"message",broken}`,
		`{"role":"assistant","content":"不应越过损坏记录"}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("broken"); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("complete corrupt record must fail with its line number, got %v", err)
	}
}

func TestFailedAppendDoesNotMutateSessionMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	beforeUpdated := sess.UpdatedAt
	beforeTitle := sess.Title()
	restore := blockSessionJournal(t, filepath.Join(dir, "default.jsonl"))
	if err := sess.Append(agent.UserMessage("不能进入内存")); err == nil {
		restore()
		t.Fatal("append should fail while journal path is blocked")
	}
	if got := sess.GetMessages(); len(got) != 0 {
		restore()
		t.Fatalf("failed append polluted messages: %#v", got)
	}
	if got := sess.Title(); got != beforeTitle {
		restore()
		t.Fatalf("failed append changed title: got=%q want=%q", got, beforeTitle)
	}
	if !sess.UpdatedAt.Equal(beforeUpdated) {
		restore()
		t.Fatalf("failed append changed UpdatedAt: got=%s want=%s", sess.UpdatedAt, beforeUpdated)
	}
	restore()
}

func TestFailedDisplayPatchDoesNotMutateSessionMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID: "call-1", Role: "tool_call", Name: "write", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	beforeUpdated := sess.UpdatedAt
	restore := blockSessionJournal(t, filepath.Join(dir, "default.jsonl"))
	if err := sess.UpdateDisplayToolResult("call-1", "write", "success", "不应进入内存"); err == nil {
		restore()
		t.Fatal("display patch should fail while journal path is blocked")
	}
	history := sess.History()
	if len(history) != 1 || history[0].Status != "running" || history[0].Result != "" {
		restore()
		t.Fatalf("failed display patch polluted memory: %#v", history)
	}
	if !sess.UpdatedAt.Equal(beforeUpdated) {
		restore()
		t.Fatalf("failed display patch changed UpdatedAt: got=%s want=%s", sess.UpdatedAt, beforeUpdated)
	}
	restore()
}

func blockSessionJournal(t *testing.T, path string) func() {
	t.Helper()
	backup := path + ".test-backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		_ = os.Rename(backup, path)
		t.Fatal(err)
	}
	restored := false
	return func() {
		t.Helper()
		if restored {
			return
		}
		restored = true
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backup, path); err != nil {
			t.Fatal(err)
		}
	}
}
