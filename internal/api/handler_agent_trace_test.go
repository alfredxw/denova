package api

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	workspacelayout "denova/internal/workspace"
)

func TestProjectAgentRunTraceAPIDoesNotUseForegroundBook(t *testing.T) {
	application := newTestApplication(t)
	projectID := application.ProjectID()
	operation, err := application.AcquireProjectOperation(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	runsDir := operation.Layout().RunsDir()
	operation.Release()
	runID := "run-background-project"
	payload := []byte("{\"type\":\"run_created\",\"run_id\":\"run-background-project\"}\n")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, runID+".jsonl"), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(application, "0")
	created := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "Foreground Trace Book"})
	if created.Code != http.StatusOK {
		t.Fatalf("create foreground Book status = %d body=%s", created.Code, created.Body.String())
	}
	if application.ProjectID() == projectID {
		t.Fatal("test setup did not switch the foreground Book")
	}

	base := "/api/projects/" + url.PathEscape(projectID) + "/agent-runs"
	list := performJSONRequest(t, server, http.MethodGet, base, nil)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(runID)) {
		t.Fatalf("Project trace list status = %d body=%s", list.Code, list.Body.String())
	}
	detail := performJSONRequest(t, server, http.MethodGet, base+"/"+runID, nil)
	if detail.Code != http.StatusOK || !bytes.Contains(detail.Body.Bytes(), []byte(runID)) {
		t.Fatalf("Project trace detail status = %d body=%s", detail.Code, detail.Body.String())
	}
	export := ut.PerformRequest(server.engine.Engine, http.MethodGet, base+"/"+runID+"/export", nil)
	if export.Code != http.StatusOK || !bytes.Equal(export.Body.Bytes(), payload) {
		t.Fatalf("Project trace export status = %d body=%q", export.Code, export.Body.Bytes())
	}
}

func TestAgentRunTraceExportAPI(t *testing.T) {
	application := newTestApplication(t)
	runID := "run-support-export"
	payload := []byte("{\"type\":\"run_created\",\"run_id\":\"run-support-export\"}\n")
	path := workspacelayout.Path(application.Workspace(), "runs", runID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")

	apiPath := "/api/projects/" + url.PathEscape(application.ProjectID()) + "/agent-runs/" + runID + "/export"
	resp := ut.PerformRequest(server.engine.Engine, http.MethodGet, apiPath, nil)
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

func TestGlobalAgentRunTraceAPIRequiresDeveloperMode(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodGet, "/api/agent-runs?limit=100", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("global Run catalog status = %d body=%s", response.Code, response.Body.String())
	}
}
