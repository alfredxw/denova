package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestLogFullModelInputWritesUntruncatedMessages(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	t.Cleanup(func() {
		modelInputLogWG.Wait()
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
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
	modelInputLogWG.Wait()

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
	if record.Cache.MessageFingerprint == "" || record.Cache.ToolSchemaFingerprint == "" || record.Cache.SystemPromptFingerprint == "" {
		t.Fatalf("cache attribution should include message/system/tool fingerprints: %#v", record.Cache)
	}
	if len(record.Cache.ToolNames) != 1 || record.Cache.ToolNames[0] != "read_file" {
		t.Fatalf("cache attribution tool names = %#v", record.Cache.ToolNames)
	}
	if len(record.Cache.ToolFingerprints) != 1 || record.Cache.ToolFingerprints[0].Name != "read_file" || record.Cache.ToolFingerprints[0].Fingerprint == "" {
		t.Fatalf("cache attribution tool fingerprints = %#v", record.Cache.ToolFingerprints)
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

func TestModelInputLogCacheAttributionFingerprintsToolSchema(t *testing.T) {
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("hello"),
	}
	tools := []*schema.ToolInfo{
		{
			Name: "read_file",
			Desc: "Read a file",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {Type: schema.String, Desc: "File path", Required: true},
			}),
		},
	}
	firstTools := modelInputLogTools(tools)
	first := modelInputLogCacheAttribution(messages, firstTools)
	second := modelInputLogCacheAttribution(messages, modelInputLogTools(tools))
	if first.MessageFingerprint == "" || first.ToolSchemaFingerprint == "" {
		t.Fatalf("fingerprints should be populated: %#v", first)
	}
	if len(first.ToolFingerprints) != 1 || first.ToolFingerprints[0].Name != "read_file" || first.ToolFingerprints[0].Fingerprint == "" {
		t.Fatalf("per-tool fingerprints should be populated without schema details: %#v", first.ToolFingerprints)
	}
	cachePayload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cachePayload), "File path") || strings.Contains(string(cachePayload), "parameters") {
		t.Fatalf("cache attribution must not expose full tool schema: %s", string(cachePayload))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input should produce stable attribution: first=%#v second=%#v", first, second)
	}

	changedTools := modelInputLogTools([]*schema.ToolInfo{
		{
			Name: "read_file",
			Desc: "Read a file with line offsets",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path":   {Type: schema.String, Desc: "File path", Required: true},
				"offset": {Type: schema.Number, Desc: "Line offset"},
			}),
		},
	})
	changed := modelInputLogCacheAttribution(messages, changedTools)
	if changed.ToolSchemaFingerprint == first.ToolSchemaFingerprint {
		t.Fatalf("tool schema fingerprint should change when schema changes: before=%#v after=%#v", first, changed)
	}
	if changed.MessageFingerprint != first.MessageFingerprint || changed.SystemPromptFingerprint != first.SystemPromptFingerprint {
		t.Fatalf("tool-only changes should not alter message/system fingerprints: before=%#v after=%#v", first, changed)
	}
}

func TestModelInputLoggingUsesStableToolSnapshot(t *testing.T) {
	originalTools := []*schema.ToolInfo{
		{
			Name:  "read_file",
			Desc:  "Read a file",
			Extra: map[string]any{"capability": "file_read"},
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {Type: schema.String, Desc: "File path", Required: true},
			}),
		},
	}
	stableTools := cloneToolInfos(originalTools)
	originalTools[0].Desc = "mutated before provider call"
	originalTools[0].Extra["capability"] = "mutated"
	originalTools[0].ParamsOneOf = schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"path":   {Type: schema.String, Desc: "File path", Required: true},
		"offset": {Type: schema.Number, Desc: "Line offset"},
	})

	capture := &toolCaptureChatModel{}
	wrapper := &modelInputLoggingChatModel{
		inner:     capture,
		agentKind: "test_agent",
		config:    openai.ChatModelConfig{Model: "test-model"},
		tools:     stableTools,
	}
	if _, err := wrapper.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	if len(capture.tools) != 1 {
		t.Fatalf("provider tools = %#v", capture.tools)
	}
	if capture.tools[0] == stableTools[0] {
		t.Fatal("provider should receive a detached tool snapshot")
	}
	if capture.tools[0].Desc != "Read a file" || capture.tools[0].Extra["capability"] != "file_read" {
		t.Fatalf("provider should receive the stable pre-mutation tool schema: %#v", capture.tools[0])
	}
	if params, _ := capture.tools[0].ParamsOneOf.ToJSONSchema(); params == nil || params.Properties.Len() != 1 {
		t.Fatalf("provider tool schema should not include later mutations: %#v", params)
	}

	capture.tools[0].Desc = "provider mutated schema"
	if _, err := wrapper.Generate(context.Background(), []*schema.Message{schema.UserMessage("again")}); err != nil {
		t.Fatal(err)
	}
	if capture.tools[0].Desc != "Read a file" {
		t.Fatalf("provider mutation should not leak into subsequent calls: %#v", capture.tools[0])
	}
}

type toolCaptureChatModel struct {
	tools []*schema.ToolInfo
}

func (m *toolCaptureChatModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	common := model.GetCommonOptions(&model.Options{}, opts...)
	m.tools = common.Tools
	return schema.AssistantMessage("ok", nil), nil
}

func (m *toolCaptureChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, io.EOF
}

func TestLogModelProviderRequestIDUpdatesModelInputRecord(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	t.Cleanup(func() {
		modelInputLogWG.Wait()
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
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
	modelInputLogWG.Wait()

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2\n%s", len(lines), string(payload))
	}
	var provider modelInputLogProviderRequestIDRecord
	if err := json.Unmarshal(lines[1], &provider); err != nil {
		t.Fatalf("unmarshal provider request id log: %v", err)
	}
	if provider.Type != "llm_provider_request_id" || provider.CallID != callID || provider.ProviderID != "req-provider-123" {
		t.Fatalf("provider request id event was not persisted: %#v", provider)
	}
}

func TestLogModelProviderRequestIDWithoutCallIDDoesNotAttachInputRecord(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	t.Cleanup(func() {
		modelInputLogWG.Wait()
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
	})

	callID := logFullModelInput(modelInputLogOptions{
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
	logModelProviderRequestIDForCall(callID, "main_agent", "adk", "response", "", "run-1", 1, msg)
	modelInputLogWG.Wait()

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2\n%s", len(lines), string(payload))
	}
	var input modelInputLogRecord
	if err := json.Unmarshal(lines[0], &input); err != nil {
		t.Fatalf("unmarshal model input log: %v", err)
	}
	var provider modelInputLogProviderRequestIDRecord
	if err := json.Unmarshal(lines[1], &provider); err != nil {
		t.Fatalf("unmarshal provider request id log: %v", err)
	}
	if provider.Type != "llm_provider_request_id" || provider.CallID != input.CallID || provider.ProviderID != "req-adk-456" {
		t.Fatalf("provider request id should attach only through explicit call id: input=%#v provider=%#v", input, provider)
	}
}

func TestLogModelProviderRequestIDKeepsExplicitConcurrentCallMapping(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(true)
	t.Cleanup(func() {
		modelInputLogWG.Wait()
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
	})

	firstCallID := logFullModelInput(modelInputLogOptions{
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
	secondCallID := logFullModelInput(modelInputLogOptions{
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

	firstMsg := schema.AssistantMessage("first response", nil)
	firstMsg.Extra = map[string]any{"openai-request-id": "req-first"}
	logModelProviderRequestIDForCall(firstCallID, "main_agent", "adk", "response", "", "run-1", 1, firstMsg)
	msg := schema.AssistantMessage("second response", nil)
	msg.Extra = map[string]any{"openai-request-id": "req-second"}
	logModelProviderRequestIDForCall(secondCallID, "main_agent", "adk", "response", "", "run-1", 2, msg)
	modelInputLogWG.Wait()

	payload, err := os.ReadFile(modelInputLogPath)
	if err != nil {
		t.Fatalf("read model input log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte{'\n'})
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4\n%s", len(lines), string(payload))
	}
	var first modelInputLogRecord
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("unmarshal first model input log: %v", err)
	}
	var second modelInputLogRecord
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("unmarshal second model input log: %v", err)
	}
	var firstProvider modelInputLogProviderRequestIDRecord
	if err := json.Unmarshal(lines[2], &firstProvider); err != nil {
		t.Fatalf("unmarshal first provider request id log: %v", err)
	}
	var secondProvider modelInputLogProviderRequestIDRecord
	if err := json.Unmarshal(lines[3], &secondProvider); err != nil {
		t.Fatalf("unmarshal second provider request id log: %v", err)
	}
	if firstProvider.CallID != first.CallID || firstProvider.ProviderID != "req-first" {
		t.Fatalf("first provider request id event = %#v, want first call id %q", firstProvider, first.CallID)
	}
	if secondProvider.CallID != second.CallID || secondProvider.ProviderID != "req-second" {
		t.Fatalf("second provider request id event = %#v, want second call id %q", secondProvider, second.CallID)
	}
}

func TestLogFullModelInputSkipsWhenDisabled(t *testing.T) {
	oldPath := modelInputLogPath
	oldSeq := modelInputLogSeq.Load()
	oldEnabled := modelInputLogEnabled.Load()
	modelInputLogPath = filepath.Join(t.TempDir(), "llm-inputs.jsonl")
	modelInputLogSeq.Store(0)
	modelInputLogEnabled.Store(false)
	t.Cleanup(func() {
		modelInputLogWG.Wait()
		modelInputLogPath = oldPath
		modelInputLogSeq.Store(oldSeq)
		modelInputLogEnabled.Store(oldEnabled)
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
