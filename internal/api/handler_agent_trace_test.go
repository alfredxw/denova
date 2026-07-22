package api

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/internal/workspacepath"
)

func TestAgentRunTraceExportAPI(t *testing.T) {
	application := newTestApplication(t)
	runID := "run-support-export"
	payload := []byte("{\"type\":\"run_created\",\"run_id\":\"run-support-export\"}\n")
	path := workspacepath.Path(application.Workspace(), "runs", runID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")

	resp := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/agent-runs/"+runID+"/export", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("export status = %d body=%s", resp.Code, resp.Body.String())
	}
	if contentType := string(resp.Header().Peek("Content-Type")); !strings.HasPrefix(contentType, "application/x-ndjson") {
		t.Fatalf("content type = %q", contentType)
	}
	if disposition := string(resp.Header().Peek("Content-Disposition")); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, runID+".jsonl") {
		t.Fatalf("content disposition = %q", disposition)
	}
	if !bytes.Equal(resp.Body.Bytes(), payload) {
		t.Fatalf("response body = %q, want %q", resp.Body.Bytes(), payload)
	}
}
