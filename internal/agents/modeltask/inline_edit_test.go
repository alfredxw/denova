package modeltask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"denova/config"
)

func TestStreamInlineEditStreamsDeltasAndStripsOuterFence(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &capturedBody); err != nil {
			t.Errorf("unmarshal request body: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{"```markdown\n", "温照野缓缓按住剑柄，", "“潮汐要涨了。”", "\n```"}
		for _, part := range chunks {
			payload, _ := json.Marshal(map[string]any{
				"id":     "chatcmpl-inline",
				"object": "chat.completion.chunk",
				"model":  "mock-model",
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{"content": part},
				}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenAIModel:   "mock-model",
		OpenAIBaseURL: server.URL + "/v1",
		OpenAIAPIKey:  "test-key",
	}

	var deltas []string
	result, err := StreamInlineEdit(context.Background(), cfg, config.AgentKindIDE, InlineEditRequest{
		Mode:          InlineEditModeWriting,
		Instruction:   "让动作和语气更有压迫感",
		Selection:     "温照野说：“潮汐快涨了。”",
		Before:        "港口的夜雾越压越低。",
		After:         "远处的钟声紧跟着响起。",
		StyleGuidance: "冷峻克制，少用形容词堆砌。",
		LoreItems: []InlineEditLoreContext{{
			Name:        "温照野",
			Type:        "character",
			Description: "港城巡夜司执事，擅长潮汐剑术。",
		}},
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamInlineEdit returned error: %v", err)
	}
	if len(deltas) != 4 {
		t.Fatalf("expected 4 streamed deltas, got %d (%q)", len(deltas), deltas)
	}
	want := "温照野缓缓按住剑柄，“潮汐要涨了。”"
	if result != want {
		t.Fatalf("normalized result = %q, want %q", result, want)
	}
	if tools, exists := capturedBody["tools"]; exists && tools != nil {
		if list, ok := tools.([]any); ok && len(list) > 0 {
			t.Fatalf("expected zero tools in inline edit call, got %v", tools)
		}
	}
	var messageText strings.Builder
	if rawMessages, ok := capturedBody["messages"].([]any); ok {
		for _, item := range rawMessages {
			if msgMap, ok := item.(map[string]any); ok {
				if content, ok := msgMap["content"].(string); ok {
					messageText.WriteString(content)
					messageText.WriteByte('\n')
				}
			}
		}
	}
	bodyText := messageText.String()
	for _, needle := range []string{"<narrative_style>", "<lore_context>", "温照野", "<target_selection>", "<instruction>"} {
		if !strings.Contains(bodyText, needle) {
			t.Fatalf("request payload missing %q: %s", needle, bodyText)
		}
	}
	if strings.Contains(bodyText, "send delegate") {
		t.Fatalf("inline edit prompt should stay free of multi-Agent delegation contracts")
	}
}

func TestResolveInlineEditModelRequiresDirectConfiguration(t *testing.T) {
	_, err := ResolveInlineEditModel(&config.Config{
		OpenAIModel:   "gpt-5",
		OpenAIBaseURL: "https://api.openai.com/v1",
		OpenAIAPIKey:  "",
	}, config.AgentKindIDE)
	if !errors.Is(err, ErrInlineEditModelNotConfigured) {
		t.Fatalf("expected ErrInlineEditModelNotConfigured for missing remote key, got %v", err)
	}

	resolved, err := ResolveInlineEditModel(&config.Config{
		OpenAIModel:   "qwen3",
		OpenAIBaseURL: "http://127.0.0.1:11434/v1",
		OpenAIAPIKey:  "",
	}, config.AgentKindIDE)
	if err != nil {
		t.Fatalf("loopback endpoint without key should be allowed, got %v", err)
	}
	if resolved.Model != "qwen3" {
		t.Fatalf("resolved model = %q, want qwen3", resolved.Model)
	}
}

func TestValidateInlineEditRequestRejectsOversizedSelection(t *testing.T) {
	err := ValidateInlineEditRequest(InlineEditRequest{
		Mode:        InlineEditModeWriting,
		Instruction: "Polish",
		Selection:   strings.Repeat("字", InlineEditMaxSelectionChars+1),
	})
	if !errors.Is(err, ErrInlineEditSelectionTooLong) {
		t.Fatalf("expected ErrInlineEditSelectionTooLong, got %v", err)
	}
}
