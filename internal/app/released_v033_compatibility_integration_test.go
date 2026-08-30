package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

func TestReleasedV033WritingSessionMigratesAndContinues(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace := filepath.Join(dataRoot, "projects", "Released v0.3.3 Book")
	legacySessionsDir := filepath.Join(workspace, ".denova", "sessions")
	if err := os.MkdirAll(legacySessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "book.json"), []byte(`{"title":"Released v0.3.3 Book","author":"Writer"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "ch0001.md"), []byte("门边放着一把蓝色钥匙。"), 0o644); err != nil {
		t.Fatal(err)
	}

	legacyMain := strings.Join([]string{
		`{"type":"session","id":"v033-writing-main","title":"旧版写作会话","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:09:00Z"}`,
		`{"type":"message","created_at":"2026-01-01T00:01:00Z","message":{"role":"user","content":"废弃线索：钥匙是红色。"}}`,
		`{"type":"clear","created_at":"2026-01-01T00:02:00Z"}`,
		`{"type":"message","created_at":"2026-01-01T00:03:00Z","message":{"role":"user","content":"请读取第一章里的钥匙线索。"}}`,
		`{"type":"context_message","created_at":"2026-01-01T00:04:00Z","message":{"role":"assistant","content":"","tool_calls":[{"id":"call-v033-read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"chapters/ch0001.md\"}"}}]}}`,
		`{"type":"context_message","created_at":"2026-01-01T00:05:00Z","message":{"role":"tool","content":"门边放着一把蓝色钥匙。","tool_call_id":"call-v033-read","tool_name":"read_file"}}`,
		`{"type":"display","id":"thinking-v033","role":"thinking","content":"正在整理旧版工具结果","created_at":"2026-01-01T00:06:00Z"}`,
		`{"type":"message","created_at":"2026-01-01T00:07:00Z","message":{"role":"assistant","content":"第一章的钥匙线索已经找到。"},"run_id":"run-v033","agent_kind":"ide","agent_name":"DenovaAgent","root_agent_name":"DenovaAgent","run_path":["DenovaAgent"]}`,
		`{"type":"context_compaction","id":"compact-v033","agent_kind":"ide","epoch":1,"summary":"旧版摘要：第一章的钥匙线索已经找到。","source_start_index":0,"source_end_index":3,"source_message_count":4,"retained_turns":1,"tokens_before":1200,"tokens_after":180,"context_window_tokens":128000,"threshold":0.9,"created_at":"2026-01-01T00:08:00Z"}`,
		"",
	}, "\n")
	legacySecondary := strings.Join([]string{
		`{"type":"session","id":"v033-writing-secondary","title":"另一个旧版会话","created_at":"2025-12-31T00:00:00Z","updated_at":"2025-12-31T00:01:00Z"}`,
		`{"type":"message","created_at":"2025-12-31T00:01:00Z","message":{"role":"user","content":"另一个会话也必须保留。"}}`,
		"",
	}, "\n")
	legacyMainPath := filepath.Join(legacySessionsDir, "v033-writing-main.jsonl")
	if err := os.WriteFile(legacyMainPath, []byte(legacyMain), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacySessionsDir, "v033-writing-secondary.jsonl"), []byte(legacySecondary), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacySessionsDir, "active.json"), []byte(`{"active_id":"v033-writing-main"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyRegistry := fmt.Sprintf(`{
  "current": %q,
  "books": [{"name":"Released v0.3.3 Book","path":%q,"author":"Writer","last_opened_at":"2026-01-01T00:10:00Z"}],
  "sort_mode": "recent"
}
`, workspace, workspace)
	if err := os.WriteFile(filepath.Join(dataRoot, "books.json"), []byte(legacyRegistry), 0o600); err != nil {
		t.Fatal(err)
	}
	legacySourceBefore, err := os.ReadFile(legacyMainPath)
	if err != nil {
		t.Fatal(err)
	}

	application, err := New(ctx, &config.Config{
		OpenAIModel: "test-model", DenovaDir: dataRoot, ResumeLastWorkspace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	if application.ProjectID() == "" || application.Workspace() != workspace {
		t.Fatalf("migrated active Project id=%q workspace=%q", application.ProjectID(), application.Workspace())
	}

	metas, err := application.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("migrated session summaries = %#v", metas)
	}
	metaByID := make(map[string]session.SessionMeta, len(metas))
	for _, meta := range metas {
		metaByID[meta.ID] = meta
	}
	if !metaByID["v033-writing-main"].Active || metaByID["v033-writing-main"].Title != "旧版写作会话" {
		t.Fatalf("migrated active session metadata = %#v", metaByID["v033-writing-main"])
	}
	if metaByID["v033-writing-secondary"].Title != "另一个旧版会话" {
		t.Fatalf("migrated secondary session metadata = %#v", metaByID["v033-writing-secondary"])
	}

	active := application.Session()
	if active == nil || active.ID != "v033-writing-main" {
		t.Fatalf("migrated active session = %#v", active)
	}
	history := active.History()
	if len(history) != 5 || history[0].Content != "废弃线索：钥匙是红色。" || history[1].Type != "clear" ||
		history[2].Content != "请读取第一章里的钥匙线索。" || history[3].Role != "thinking" ||
		history[4].Content != "第一章的钥匙线索已经找到。" {
		t.Fatalf("migrated visible history = %#v", history)
	}
	effective := active.GetEffectiveMessages()
	if len(effective) != 4 || effective[0].Content != "请读取第一章里的钥匙线索。" ||
		len(effective[1].ToolCalls) != 1 || effective[2].Role != agent.ToolRole ||
		effective[2].Content != "门边放着一把蓝色钥匙。" || effective[3].Content != "第一章的钥匙线索已经找到。" {
		t.Fatalf("migrated effective context = %#v", effective)
	}

	record, err := application.projectRegistry.Get(application.ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	layout, err := application.projectRegistry.Layout(record)
	if err != nil {
		t.Fatal(err)
	}
	migratedSessionPath := filepath.Join(layout.SessionsDir(), "v033-writing-main.jsonl")
	migratedBeforeContinuation, err := os.ReadFile(migratedSessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(migratedBeforeContinuation, legacySourceBefore) {
		t.Fatal("Project state did not preserve the released session journal as its prefix")
	}
	if sourceAfterMigration, readErr := os.ReadFile(legacyMainPath); readErr != nil || !bytes.Equal(sourceAfterMigration, legacySourceBefore) {
		t.Fatalf("released session source changed during migration: err=%v", readErr)
	}

	model := &releasedV033CaptureModel{response: "钥匙是蓝色。"}
	executionRuntime := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = executionRuntime.Close(context.Background()) })
	operation, err := startPublicExecutionCycle(
		executionRuntime,
		ctx,
		model,
		agentconversation.NewSessionConversation(active),
		application.BookService(),
		agentchat.ChatRequest{CommandID: "continue-v033-session", Message: "根据旧线索继续：钥匙是什么颜色？", Locale: "zh-CN"},
		agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: application.ProjectID(), StateRoot: layout.StateRoot,
			SessionID: active.ID, Workspace: workspace, Mode: "ide",
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("continued v0.3.3 session outcome = %#v", outcome)
	}

	modelInput := model.lastInput(t)
	var modelTranscript strings.Builder
	for _, message := range modelInput {
		modelTranscript.WriteString(string(message.Role))
		modelTranscript.WriteByte(':')
		modelTranscript.WriteString(message.Content)
		modelTranscript.WriteByte('\n')
	}
	transcript := modelTranscript.String()
	for _, expected := range []string{
		"请读取第一章里的钥匙线索。", "门边放着一把蓝色钥匙。",
		"第一章的钥匙线索已经找到。", "根据旧线索继续：钥匙是什么颜色？",
	} {
		if !strings.Contains(transcript, expected) {
			t.Fatalf("continued model transcript omitted %q:\n%s", expected, transcript)
		}
	}
	if strings.Contains(transcript, "废弃线索：钥匙是红色。") {
		t.Fatalf("continued model transcript crossed the released clear marker:\n%s", transcript)
	}

	reloadedStore, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("v033-writing-main")
	if err != nil {
		t.Fatal(err)
	}
	reloadedHistory := reloaded.History()
	if len(reloadedHistory) != 8 || reloadedHistory[5].Content != "根据旧线索继续：钥匙是什么颜色？" ||
		reloadedHistory[6].Content != "钥匙是蓝色。" || reloadedHistory[7].Role != "execution_summary" {
		t.Fatalf("continued durable history = %#v", reloadedHistory)
	}
	migratedAfterContinuation, err := os.ReadFile(migratedSessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(migratedAfterContinuation, legacySourceBefore) || len(migratedAfterContinuation) <= len(migratedBeforeContinuation) {
		t.Fatal("continued session did not append after the released journal")
	}
	if sourceAfterContinuation, readErr := os.ReadFile(legacyMainPath); readErr != nil || !bytes.Equal(sourceAfterContinuation, legacySourceBefore) {
		t.Fatalf("released session source changed after continuation: err=%v", readErr)
	}
}

type releasedV033CaptureModel struct {
	mu       sync.Mutex
	response string
	input    []*agent.Message
}

func (model *releasedV033CaptureModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	model.capture(messages)
	return agent.AssistantMessage(model.response, nil), nil
}

func (model *releasedV033CaptureModel) Stream(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	model.capture(messages)
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage(model.response, nil)}), nil
}

func (model *releasedV033CaptureModel) capture(messages []*agent.Message) {
	model.mu.Lock()
	defer model.mu.Unlock()
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	model.input = cloned
}

func (model *releasedV033CaptureModel) lastInput(t *testing.T) []*agent.Message {
	t.Helper()
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.input) == 0 {
		t.Fatal("continued v0.3.3 session did not reach the model")
	}
	return model.input
}
