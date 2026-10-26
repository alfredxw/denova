package agentrun

import (
	"context"
	"testing"
	"time"

	"denova/config"
	"denova/internal/statistics"
	"github.com/alfredxw/denova/agent/model/providers"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestStatisticsCaptureWithoutDiagnosticTracingAndWithoutDuplicates(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{ProjectStoreDir: root}
	ctx, finish := WithStandaloneTrace(context.Background(), cfg, "general", "statistics-test", "generate", nil)
	defer finish(nil)
	span, id, _ := BeginLLMCallTrace(ctx, "general", "test", "generate", providers.ModelConfig{Model: "test-model"}, nil, nil, false)
	message := &agentschema.Message{ResponseMeta: &agentschema.ResponseMeta{Usage: &agentschema.TokenUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}}}
	FinishLLMCallTrace(span, id, "general", "test", "generate", "test-model", 1, message, nil, nil)
	FinishLLMCallTrace(span, id, "general", "test", "generate", "test-model", 1, message, nil, nil)
	var calls []statistics.Call
	if err := statistics.ReadCalls(root, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), func(call statistics.Call) { calls = append(calls, call) }); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != id || calls[0].Usage.Total != 120 || calls[0].Model != "test-model" {
		t.Fatalf("calls = %+v", calls)
	}
}
