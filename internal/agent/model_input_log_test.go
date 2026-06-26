package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

func TestLogFullModelInputWritesUntruncatedMessages(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	oldPending := modelInputLogPending
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	modelInputLogPending = map[modelInputLogPendingKey][]string{}
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
		modelInputLogPending = oldPending
	})

	longContent := strings.Repeat("完整输入", 12000)
	logFullModelInput(modelInputLogOptions{
		AgentKind: "test_agent",
		Source:    "test",
		Mode:      "generate",
		Config: openai.ChatModelConfig{
			APIKey:  "secret-key-must-not-be-logged",
			Model:   "test-model",
			BaseURL: "https://example.test/v1",
		},
		Messages: []*schema.Message{
			schema.SystemMessage("system"),
			schema.UserMessage(longContent),
		},
		Tools: []*schema.ToolInfo{
			{
				Name: "read_file",
				Desc: "Read a file",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path": {Type: schema.String, Desc: "File path", Required: true},
				}),
			},
		},
	})

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	if strings.Contains(string(payload), "secret-key-must-not-be-logged") {
		t.Fatal("model input log must not include API keys")
	}

	var record modelInputLogRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatalf("unmarshal model input log: %v", err)
	}
	if record.MessageCount != 2 || len(record.Messages) != 2 {
		t.Fatalf("unexpected messages count: count=%d len=%d", record.MessageCount, len(record.Messages))
	}
	if record.ToolCount != 1 || len(record.Tools) != 1 {
		t.Fatalf("unexpected tools count: count=%d len=%d", record.ToolCount, len(record.Tools))
	}
	if record.Tools[0].Parameters == nil {
		t.Fatal("tool parameters schema was not logged")
	}
	if got := record.Messages[1].Content; got != longContent {
		t.Fatalf("message content was not preserved: got_len=%d want_len=%d", len(got), len(longContent))
	}
	if record.ModelConfig.Model != "test-model" || record.ModelConfig.BaseURL != "https://example.test/v1" {
		t.Fatalf("unexpected model metadata: %#v", record.ModelConfig)
	}
}

func TestLogModelProviderRequestIDUpdatesModelInputRecord(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	oldPending := modelInputLogPending
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	modelInputLogPending = map[modelInputLogPendingKey][]string{}
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
		modelInputLogPending = oldPending
	})

	callID := logFullModelInput(modelInputLogOptions{
		AgentKind: "test_agent",
		Source:    "test",
		Mode:      "generate",
		Config: openai.ChatModelConfig{
			Model: "test-model",
		},
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
		},
	})
	if callID == "" {
		t.Fatal("expected model input call id")
	}
	msg := schema.AssistantMessage("world", nil)
	msg.Extra = map[string]any{"openai-request-id": " req-provider-123 "}

	got := logModelProviderRequestIDForCall(callID, "test_agent", "test", "generate", "test-model", "", 0, msg)
	if got != "req-provider-123" {
		t.Fatalf("provider request id = %q, want req-provider-123", got)
	}

	var record modelInputLogRecord
	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatalf("unmarshal model input log: %v", err)
	}
	if record.ProviderID != "req-provider-123" {
		t.Fatalf("provider request id was not persisted: %#v", record)
	}
}

func TestLogModelProviderRequestIDUsesPendingModelInputRecord(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	oldPending := modelInputLogPending
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	modelInputLogPending = map[modelInputLogPendingKey][]string{}
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
		modelInputLogPending = oldPending
	})

	logFullModelInput(modelInputLogOptions{
		AgentKind: "main_agent",
		Source:    "adk",
		Mode:      "stream",
		Config: openai.ChatModelConfig{
			Model: "test-model",
		},
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
		},
	})
	msg := schema.AssistantMessage("world", nil)
	msg.Extra = map[string]any{"openai-request-id": "req-adk-456"}

	logModelProviderRequestID("main_agent", "adk", "response", "", "run-1", 1, msg)

	var record modelInputLogRecord
	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatalf("unmarshal model input log: %v", err)
	}
	if record.ProviderID != "req-adk-456" {
		t.Fatalf("provider request id was not persisted from pending call: %#v", record)
	}
}

func TestLogModelProviderRequestIDWithoutIDConsumesPendingModelInputRecord(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	oldPending := modelInputLogPending
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	modelInputLogPending = map[modelInputLogPendingKey][]string{}
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
		modelInputLogPending = oldPending
	})

	logFullModelInput(modelInputLogOptions{
		AgentKind: "main_agent",
		Source:    "adk",
		Mode:      "stream",
		Config: openai.ChatModelConfig{
			Model: "test-model",
		},
		Messages: []*schema.Message{
			schema.UserMessage("first"),
		},
	})
	logFullModelInput(modelInputLogOptions{
		AgentKind: "main_agent",
		Source:    "adk",
		Mode:      "stream",
		Config: openai.ChatModelConfig{
			Model: "test-model",
		},
		Messages: []*schema.Message{
			schema.UserMessage("second"),
		},
	})

	logModelProviderRequestID("main_agent", "adk", "response", "", "run-1", 1, schema.AssistantMessage("first response", nil))
	msg := schema.AssistantMessage("second response", nil)
	msg.Extra = map[string]any{"openai-request-id": "req-second"}
	logModelProviderRequestID("main_agent", "adk", "response", "", "run-1", 2, msg)

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
	var first modelInputLogRecord
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("unmarshal first model input log: %v", err)
	}
	var second modelInputLogRecord
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("unmarshal second model input log: %v", err)
	}
	if first.ProviderID != "" {
		t.Fatalf("first provider request id = %q, want empty", first.ProviderID)
	}
	if second.ProviderID != "req-second" {
		t.Fatalf("second provider request id = %q, want req-second", second.ProviderID)
	}
}

func TestLogFullModelInputSkipsWhenDisabled(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	oldPending := modelInputLogPending
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(false)
	modelInputLogPending = map[modelInputLogPendingKey][]string{}
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
		modelInputLogPending = oldPending
	})

	logFullModelInput(modelInputLogOptions{
		AgentKind: "test_agent",
		Source:    "test",
		Mode:      "generate",
		Config: openai.ChatModelConfig{
			Model: "test-model",
		},
		Messages: []*schema.Message{
			schema.UserMessage("hidden unless dev mode is enabled"),
		},
	})

	if _, err := os.Stat(modelInputLogPath); !os.IsNotExist(err) {
		t.Fatalf("model input log should not be created when disabled: %v", err)
	}
	if got := modelInputLogSeq.Load(); got != 0 {
		t.Fatalf("model input log sequence advanced while disabled: got %d", got)
	}
}

func TestAppendModelInputLogKeepsOnlyRecentLines(t *testing.T) {
	oldPath := modelInputLogPath
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	t.Cleanup(func() {
		modelInputLogPath = oldPath
	})

	for i := 0; i < 12; i++ {
		if err := appendModelInputLog([]byte(fmt.Sprintf("{\"seq\":%d}\n", i))); err != nil {
			t.Fatalf("append model input log %d: %v", i, err)
		}
	}

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte{'\n'})
	if len(lines) != modelInputLogMaxLines {
		t.Fatalf("line count = %d, want %d\n%s", len(lines), modelInputLogMaxLines, string(payload))
	}
	if !bytes.Contains(lines[0], []byte(`"seq":2`)) || !bytes.Contains(lines[len(lines)-1], []byte(`"seq":11`)) {
		t.Fatalf("unexpected retained range: first=%s last=%s", lines[0], lines[len(lines)-1])
	}
}
