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
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	t.Cleanup(func() {
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
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
