package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	apptask "denova/internal/app/task"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

func TestWritingLegacyCheckpointPreservesCompactionSummaryOnContinuation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var (
		mu              sync.Mutex
		capturedBodies  []string
		streamResponses = []string{
			"ARCHIVED_ASSISTANT_RESPONSE",
			"RETAINED_RECENT_ANSWER",
			"CONTINUED_AFTER_UPGRADE",
			"CONTINUED_AFTER_MIGRATION",
		}
		streamIndex int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Stream   bool              `json:"stream"`
			Messages []json.RawMessage `json:"messages"`
		}
		rawBody := new(bytes.Buffer)
		if _, err := rawBody.ReadFrom(r.Body); err != nil {
			t.Error(err)
			return
		}
		if err := json.Unmarshal(rawBody.Bytes(), &payload); err != nil {
			t.Error(err)
			return
		}
		isCompaction := strings.Contains(rawBody.String(), "[Runtime context compaction request]")
		if !payload.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chatcmpl-compact","created":1700000001,"model":"test-model","choices":[{"message":{"role":"assistant","content":"Legacy writing summary: keep blue key."},"finish_reason":"stop"}]}`)
			return
		}
		reply := "OK"
		idx := 0
		if isCompaction {
			reply = "Legacy writing summary: keep blue key."
		} else {
			mu.Lock()
			capturedBodies = append(capturedBodies, rawBody.String())
			if streamIndex < len(streamResponses) {
				reply = streamResponses[streamIndex]
				streamIndex++
			}
			idx = streamIndex
			mu.Unlock()
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-request-id", fmt.Sprintf("req-writing-%d", idx))
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-writing-%d\",\"created\":170000000%d,\"model\":\"test-model\",\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", idx, idx, reply)
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-writing-%d\",\"created\":170000000%d,\"model\":\"test-model\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n", idx, idx)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	t.Setenv("OPENAI_API_KEY", "fixture-only")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	booksRoot := filepath.Join(root, "books")
	if err := config.WriteSettingsFile(config.UserConfigPath(dataDir), config.Settings{
		OpenAIModel:   "test-model",
		OpenAIAPIKey:  "fixture-only",
		OpenAIBaseURL: server.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	newApp := func() *App {
		application, err := New(ctx, &config.Config{
			DenovaDir:           dataDir,
			OpenAIModel:         "test-model",
			OpenAIAPIKey:        "fixture-only",
			OpenAIBaseURL:       server.URL + "/v1",
			ResumeLastWorkspace: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return application
	}

	firstApp := newApp()
	created, err := firstApp.CreateBook(ctx, booksRoot, "Legacy Writing Compaction", "Writer", "", "")
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}

	runWritingTurn := func(application *App, commandID, message string) {
		t.Helper()
		task, err := application.StartTaskWithError(ctx, agentchat.ChatRequest{
			CommandID: commandID,
			Message:   message,
		})
		if err != nil {
			t.Fatalf("StartTaskWithError(%s): %v", commandID, err)
		}
		select {
		case <-task.Done():
		case <-ctx.Done():
			t.Fatalf("timed out waiting for writing turn %s", commandID)
		}
		if status := task.Status(); status != apptask.Done {
			t.Fatalf("writing turn %s status = %s, snap = %+v", commandID, status, task.Snapshot())
		}
	}

	archivedProse := strings.Repeat("ARCHIVED_SECRET_LINE ", 1500)
	runWritingTurn(firstApp, "legacy-turn-1", archivedProse)
	runWritingTurn(firstApp, "legacy-turn-2", "Keep this recent turn.")

	compactionResult, err := firstApp.CompactContextCommand(ctx, "legacy-compact")
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	if !compactionResult.Triggered {
		firstApp.Close()
		t.Fatal("expected writing compaction to trigger")
	}

	firstApp.mu.RLock()
	sessionID := firstApp.session.ID
	firstApp.mu.RUnlock()
	record, err := firstApp.projectRegistry.Get(created.ProjectID)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	layout, err := firstApp.projectRegistry.Layout(record)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	firstApp.Close()

	// Append a legacy-format session.message_checkpoint (no "version" field,
	// Hash computed with HashCanonical including provider telemetry Extra)
	// matching what the released baseline persisted for the exact same messages.
	sessionStore, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sessionStore.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	canonicalMsgs, err := sess.ReadCanonicalMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ordered := agentcanonical.CanonicalContextStateOrder(canonicalMsgs)
	if err := sessionStore.Close(); err != nil {
		t.Fatal(err)
	}

	agentKey, err := agentrun.AgentSessionKeyForOptions(agentrun.Options{
		AgentKind: agentrun.AgentKindIDE,
		ProjectID: created.ProjectID,
		StateRoot: layout.StoreRoot,
		SessionID: sessionID,
		Mode:      "ide",
	})
	if err != nil {
		t.Fatal(err)
	}
	agentStore, err := canonicalstore.New(dataDir, firstApp.projectRegistry)
	if err != nil {
		t.Fatal(err)
	}
	agentLog, err := agentStore.Open(ctx, agentKey)
	if err != nil {
		t.Fatal(err)
	}
	var (
		revision agentsession.Revision
		latestCP json.RawMessage
	)
	if _, err := agentLog.Replay(ctx, func(rec agentsession.Record) error {
		revision = rec.Revision
		if rec.Kind == "session.message_checkpoint" {
			latestCP = append(json.RawMessage(nil), rec.Data...)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(latestCP) == 0 {
		t.Fatal("did not find session.message_checkpoint record in writing journal")
	}

	var cp struct {
		Archive      *agenthistory.HistoryArchive `json:"archive,omitempty"`
		Hash         string                       `json:"hash"`
		MessageCount int                          `json:"message_count"`
	}
	if err := json.Unmarshal(latestCP, &cp); err != nil {
		t.Fatal(err)
	}
	selected, err := cp.Archive.SelectMessages(ordered[:cp.MessageCount])
	if err != nil {
		t.Fatal(err)
	}
	legacyHash, err := agentschema.HashCanonical(selected)
	if err != nil {
		t.Fatal(err)
	}
	strippedHash, err := agentschema.HashCanonicalMessages(selected)
	if err != nil {
		t.Fatal(err)
	}
	if legacyHash == strippedHash {
		t.Fatal("expected retained writing messages to include provider telemetry so legacy and current hashes differ")
	}
	var cpMap map[string]any
	if err := json.Unmarshal(latestCP, &cpMap); err != nil {
		t.Fatal(err)
	}
	delete(cpMap, "version")
	cpMap["hash"] = legacyHash
	legacyData, err := json.Marshal(cpMap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentLog.Append(ctx, revision, agentsession.Record{
		Kind:    "session.message_checkpoint",
		Version: 1,
		Data:    legacyData,
	}); err != nil {
		t.Fatal(err)
	}
	if err := agentLog.Close(); err != nil {
		t.Fatal(err)
	}

	journalPath := filepath.Join(layout.SessionsDir(), sessionID+".jsonl")
	_ = os.Remove(filepath.Join(layout.SessionsDir(), sessionID+".idx.json"))

	// Reopen with the current version, verify read-only inspection keeps the
	// released journal untouched and retains the summary, then continue writing.
	secondApp := newApp()
	journalBeforeInspect, err := os.ReadFile(journalPath)
	if err != nil {
		secondApp.Close()
		t.Fatal(err)
	}
	analysis, err := secondApp.AnalyzeContext(ctx, agentchat.ChatRequest{Message: "Inspect context."})
	if err != nil {
		secondApp.Close()
		t.Fatal(err)
	}
	if analysis.Compaction == nil || analysis.Compaction.Summary != "Legacy writing summary: keep blue key." {
		secondApp.Close()
		t.Fatalf("read-only inspection lost legacy compaction summary: %+v", analysis.Compaction)
	}
	journalAfterInspect, err := os.ReadFile(journalPath)
	if err != nil {
		secondApp.Close()
		t.Fatal(err)
	}
	if !bytes.Equal(journalBeforeInspect, journalAfterInspect) {
		secondApp.Close()
		t.Fatal("read-only inspection mutated the released writing journal")
	}

	runWritingTurn(secondApp, "continue-after-upgrade", "Continue writing chapter 2.")
	secondApp.Close()

	mu.Lock()
	continuedRequest := capturedBodies[2]
	mu.Unlock()
	if !strings.Contains(continuedRequest, "Legacy writing summary: keep blue key.") {
		t.Fatalf("continuation model request omitted legacy compaction summary:\n%s", continuedRequest)
	}
	if strings.Contains(continuedRequest, "ARCHIVED_SECRET_LINE") {
		t.Fatalf("continuation model request resent archived prose:\n%s", continuedRequest)
	}
	if !strings.Contains(continuedRequest, "RETAINED_RECENT_ANSWER") {
		t.Fatalf("continuation model request omitted retained recent turn:\n%s", continuedRequest)
	}

	// Continuation must migrate the persisted checkpoint to the current version.
	verifyStore, err := canonicalstore.New(dataDir, secondApp.projectRegistry)
	if err != nil {
		t.Fatal(err)
	}
	verifyLog, err := verifyStore.Open(ctx, agentKey)
	if err != nil {
		t.Fatal(err)
	}
	latestVersion := -1
	if _, err := verifyLog.Replay(ctx, func(rec agentsession.Record) error {
		if rec.Kind == "session.message_checkpoint" {
			var decoded map[string]any
			if err := json.Unmarshal(rec.Data, &decoded); err != nil {
				return err
			}
			v, _ := decoded["version"].(float64)
			latestVersion = int(v)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := verifyLog.Close(); err != nil {
		t.Fatal(err)
	}
	if latestVersion != 1 {
		t.Fatalf("expected migrated checkpoint version 1 after continuation, got %d", latestVersion)
	}

	// A third cold reopen continues seamlessly with the migrated checkpoint.
	thirdApp := newApp()
	runWritingTurn(thirdApp, "continue-after-migration", "Continue writing chapter 3.")
	thirdApp.Close()
	mu.Lock()
	postMigrationRequest := capturedBodies[3]
	mu.Unlock()
	if !strings.Contains(postMigrationRequest, "Legacy writing summary: keep blue key.") || strings.Contains(postMigrationRequest, "ARCHIVED_SECRET_LINE") {
		t.Fatalf("post-migration continuation lost summary or resent archived prose:\n%s", postMigrationRequest)
	}
}
