package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/external/journal"
	agent "github.com/alfredxw/denova/agent"
)

func TestExternalJournalAcceptanceToolsRestartAndAtomicCompletion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "外部 创作.jsonl")
	sess, err := createSessionWithRuntimeConfig("外部 创作", path, "External writing", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	native := testRuntimeConfig(config.AgentKindIDE)
	initial, err := sess.EnsureRuntimeConfig(native)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	externalConfig := native
	externalConfig.Runtime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture-model"}}
	selected, err := sess.SetRuntimeConfig(externalConfig, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".pre-external-runtime-v1.bak")
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatalf("runtime upgrade did not preserve the original: %v", err)
	}
	selected.Runtime.Codex.Model = "mutated-draft"
	selected, _ = sess.RuntimeConfig()
	if selected.Runtime.Codex.Model != "fixture-model" {
		t.Fatal("snapshot draft mutated the stored model")
	}
	revision := selected.Revision
	record := func(kind externaljournal.Kind, operation string, data any) externaljournal.Record {
		t.Helper()
		value, err := externaljournal.NewRecord(kind, operation, revision, data)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	commit := func(change ExternalTransaction) error {
		return sess.UpdateExternal(ctx, revision, func(ExternalState) (ExternalTransaction, error) { return change, nil })
	}
	accept := record(externaljournal.OperationAccepted, "op-1", externaljournal.Accepted{CommandID: "command-1", Fingerprint: "fingerprint-1", Runtime: selected.Engine(), InputMessageID: "input-1"})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{accept}}); err == nil {
		t.Fatal("acceptance committed without its input")
	}
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{accept}, Message: agent.UserMessage("Revise the opening."), Metadata: MessageMetadata{MessageID: "input-1", AgentOperationID: "op-1", AgentCommandID: "command-1"}}); err != nil {
		t.Fatal(err)
	}
	start := record(externaljournal.ToolStarted, "op-1", externaljournal.StartedTool{ExecutionID: "tool-1", Tool: "ask", Recovery: externaljournal.ReadOnly, Arguments: json.RawMessage(`{"questions":[{"id":"tone","prompt":"private-question-token"}]}`)})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{start}}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.SetRuntimeConfig(native, revision); !errors.Is(err, externaljournal.ErrBusy) {
		t.Fatalf("runtime switched during a pending question: %v", err)
	}
	closed := record(externaljournal.OperationClosed, "op-1", externaljournal.Closed{Status: externaljournal.Completed, MessageID: "output-1"})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{closed}, Message: agent.AssistantMessage("Premature completion", nil), Metadata: MessageMetadata{MessageID: "output-1"}}); err == nil {
		t.Fatal("completed while a tool was unsettled")
	}
	interrupted := record(externaljournal.OperationClosed, "op-1", externaljournal.Closed{Status: externaljournal.Interrupted})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{interrupted}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	indexPath := path[:len(path)-len(".jsonl")] + ".idx.json"
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(index, []byte("private-question-token")) {
		t.Fatal("rebuildable index copied private tool content")
	}
	if err := os.Remove(indexPath); err != nil {
		t.Fatal(err)
	}
	sess, err = loadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.ReadExternal(ctx, func(state ExternalState) error {
		op := state.Projection.Operations["op-1"]
		if op == nil || op.Status != externaljournal.Interrupted || op.Tools["tool-1"].Finished != nil {
			t.Fatalf("pending question did not survive replay: %#v", op)
		}
		question, err := state.Read(op.Tools["tool-1"].Started)
		if err != nil {
			return err
		}
		if !bytes.Contains(question.Data, []byte("private-question-token")) {
			t.Fatal("question source was lost")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	answer := record(externaljournal.ToolFinished, "op-1", externaljournal.FinishedTool{ExecutionID: "tool-1", Success: true, Result: `{"schema":"ask.result.v1","id":"ask-tool-1","status":"answered","answers":[{"question_id":"tone","custom_input":"Restrained"}]}`})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{answer}}); err != nil {
		t.Fatal(err)
	}
	continued := record(externaljournal.OperationAccepted, "op-2", externaljournal.Accepted{CommandID: "command-2", Fingerprint: "fingerprint-2", Runtime: selected.Engine(), InputMessageID: "input-2", ContinuesOperationID: "op-1"})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{continued}, Message: agent.UserMessage("Continue using the saved answer."), Metadata: MessageMetadata{MessageID: "input-2"}}); err != nil {
		t.Fatal(err)
	}
	final := record(externaljournal.OperationClosed, "op-2", externaljournal.Closed{Status: externaljournal.Completed, MessageID: "output-2", AgentKind: config.AgentKindIDE, Usage: &agent.TokenUsage{PromptTokens: 200, CompletionTokens: 50, TotalTokens: 250}})
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{final}, Message: agent.AssistantMessage("Confirmed final text.", nil), Metadata: MessageMetadata{MessageID: "output-2"}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.ReadExternal(ctx, func(state ExternalState) error {
		op := state.Projection.Operations["op-2"]
		transaction, err := sess.journal.ReadRange(ctx, conversationjournal.Range{After: op.Closed.Cursor - 1, Through: op.Closed.Cursor})
		if err != nil {
			return err
		}
		if len(transaction) != 2 || !bytes.Contains(transaction[0].Payload, []byte("Confirmed final text.")) || !bytes.Contains(transaction[1].Payload, []byte("operation.closed")) {
			t.Fatal("final text and operation closure were not atomic")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.SetRuntimeConfig(native, revision); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = loadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	usageRows := 0
	for _, entry := range sess.records {
		if entry.display != nil && entry.display.Role == "token_usage" {
			usageRows++
			if entry.display.TotalTokens != 250 || entry.display.ModelCalls != 0 {
				t.Fatalf("invalid restored usage: %+v", entry.display)
			}
		}
	}
	if usageRows != 1 {
		t.Fatalf("restored %d usage rows", usageRows)
	}
	if _, _, ok := sess.LatestModelPromptUsage(config.AgentKindIDE); ok {
		t.Fatal("aggregate engine usage calibrated a Native request")
	}
	if err := commit(ExternalTransaction{Records: []externaljournal.Record{answer}}); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("stale result crossed configuration boundary: %v", err)
	}
	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := sess.ReadExternal(ctx, func(state ExternalState) error {
		if len(state.Projection.Operations) != 0 {
			t.Fatal("clear left old operation handles active")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExternalRuntimeSnapshotHasReleasedReaderGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "external.jsonl")
	selection := testRuntimeConfig(config.AgentKindGeneral)
	selection.Runtime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture-model"}}
	sess, err := createSessionWithRuntimeConfig("external", path, "External", &selection)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	restored, ok := sess.RuntimeConfig()
	if !ok || !reflect.DeepEqual(restored.Config, selection) {
		t.Fatalf("external initial snapshot was lost: %#v", restored)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(content), []byte{'\n'})
	var header sessionHeader
	if err := json.Unmarshal(lines[0], &header); err != nil {
		t.Fatal(err)
	}
	if header.RuntimeConfig != nil || len(lines) != 2 || !bytes.Contains(lines[1], []byte(`"type":"session_patch_v2"`)) {
		t.Fatal("released readers could silently interpret an external snapshot as Native")
	}
}
