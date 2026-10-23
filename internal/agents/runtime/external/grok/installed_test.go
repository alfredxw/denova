package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the released executable against a local model fixture. This proves
// the headless channel, its private configuration and session resume without
// account credentials.
func TestInstalledGrokHeadless(t *testing.T) {
	executable := os.Getenv("DENOVA_TEST_GROK")
	if executable == "" {
		t.Skip("set DENOVA_TEST_GROK to exercise an installed Grok Build binary")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"denova-test"}]}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(value any) {
			if object, ok := value.(map[string]any); ok {
				object["created"] = 1
				object["model"] = "denova-test"
			}
			body, _ := json.Marshal(value)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
		}
		write(map[string]any{"id": "completion-1", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"role": "assistant", "content": "Headless OK."}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	home := t.TempDir()
	configPath := filepath.Join(home, "config.toml")
	config := fmt.Sprintf("[model.denova-test]\nmodel = \"denova-test\"\nname = \"Denova Test\"\nbase_url = %q\napi_key = \"fixture-only\"\napi_backend = \"chat_completions\"\ncontext_window = 128000\n[models]\ndefault = \"denova-test\"\n", server.URL+"/v1")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := Connect(ctx, ProcessOptions{Executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// The attempt keeps its own private home; only model routing is imported.
	client.env = append(client.env, "GROK_CONFIG_PATH="+configPath, "GROK_AUTH_PATH="+filepath.Join(home, "auth.json"))
	input := fixtureInput(t)
	input.Selection.Grok.Model = "denova-test"
	input.Selection.Grok.ContextWindow = 128000
	host := &recordingHost{}
	result, err := client.Run(ctx, input, host)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Settled || !strings.Contains(result.Text, "Headless OK") || result.SessionID == "" {
		t.Fatalf("real CLI headless result: %+v", result)
	}
	input.SessionID = result.SessionID
	resumed, err := client.Run(ctx, input, &recordingHost{})
	if err != nil {
		t.Fatalf("real CLI resume: %v", err)
	}
	if !resumed.Settled || resumed.SessionID != result.SessionID {
		t.Fatalf("real CLI resumed result: %+v", resumed)
	}
}
