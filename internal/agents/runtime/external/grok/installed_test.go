package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// installedGrok returns the released executable selected by DENOVA_TEST_GROK,
// skipping the test when the variable is absent.
func installedGrok(t *testing.T) string {
	t.Helper()
	executable := os.Getenv("DENOVA_TEST_GROK")
	if executable == "" {
		t.Skip("set DENOVA_TEST_GROK to exercise an installed Grok Build binary")
	}
	return executable
}

// modelFixture serves a fake completion for the `denova-test` model; capture
// receives the /v1/chat/completions request body for assertion. It returns the
// server (auto-closed) and the config path for the imported model routing.
func modelFixture(t *testing.T, reply string, capture func(payload []byte)) (*httptest.Server, string) {
	t.Helper()
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
		if capture != nil {
			payload, _ := io.ReadAll(r.Body)
			capture(payload)
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
		write(map[string]any{"id": "completion", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"role": "assistant", "content": reply}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	home := t.TempDir()
	configPath := filepath.Join(home, "config.toml")
	config := fmt.Sprintf("[model.denova-test]\nmodel = \"denova-test\"\nname = \"Denova Test\"\nbase_url = %q\napi_key = \"fixture-only\"\napi_backend = \"chat_completions\"\ncontext_window = 128000\n[models]\ndefault = \"denova-test\"\n", server.URL+"/v1")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return server, configPath
}

// installedInput returns a connected client pinned to the fake model and the
// input shape a product turn would use.
func installedInput(t *testing.T, configPath string) (*Client, external.Input) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	client, err := Connect(ctx, ProcessOptions{Executable: installedGrok(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	// The attempt keeps its own private home; only model routing is imported.
	client.env = append(client.env, "GROK_CONFIG_PATH="+configPath, "GROK_AUTH_PATH="+filepath.Join(filepath.Dir(configPath), "auth.json"))
	input := fixtureInput(t)
	input.Selection.Grok.Model = "denova-test"
	input.Selection.Grok.ContextWindow = 128000
	return client, input
}

// Exercise the released executable against a local model fixture. This proves
// the headless channel, its private configuration and session resume without
// account credentials.
func TestInstalledGrokHeadless(t *testing.T) {
	_, configPath := modelFixture(t, "Headless OK.", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, input := installedInput(t, configPath)
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

// An image turn against the released executable must reach the model server:
// the picture travels inside the .json prompt file, not the command line, and
// the CLI parses the blocks far enough to request the vision-capable turn.
func TestInstalledGrokImageTurn(t *testing.T) {
	var visionPayload []byte
	_, configPath := modelFixture(t, "Image read.", func(payload []byte) { visionPayload = payload })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, input := installedInput(t, configPath)
	input.Attachments = []agentschema.Attachment{fixtureAttachment(t, 512, 512)}
	result, err := client.Run(ctx, input, &recordingHost{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Settled || !strings.Contains(result.Text, "Image read") {
		t.Fatalf("real CLI image turn result: %+v", result)
	}
	// The model request must carry the picture: the CLI read the image block from
	// the .json prompt file and attached it to the vision-able API call as a
	// session asset reference, never as an inline argument.
	if len(visionPayload) == 0 {
		t.Fatal("model server received no vision request")
	}
	for _, marker := range []string{"<image_files>", "assets/", "image-"} {
		if strings.Contains(string(visionPayload), marker) {
			return
		}
	}
	t.Fatalf("model request carried no image reference: %s", visionPayload)
}