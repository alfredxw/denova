package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	runtimeapp "denova/internal/app"
	booklore "denova/internal/book/lore"
)

func TestInlineEditStreamRoundtripAndSessionIsolation(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, delta := range []string{"夜风掠过栈桥，", "温照野低声道：“潮汐将至。”"} {
			payload, _ := json.Marshal(map[string]any{
				"id":     "chatcmpl-api-inline",
				"object": "chat.completion.chunk",
				"model":  "mock-inline",
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{"content": delta},
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
	defer modelServer.Close()

	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "mock-inline",
		OpenAIBaseURL:       modelServer.URL + "/v1",
		OpenAIAPIKey:        "test-key",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	projectID := application.ProjectID()
	enabled := true
	if _, err := booklore.NewStore(application.Workspace()).Create(booklore.ItemInput{
		ID:               "wen-zhaoye",
		Name:             "温照野",
		Type:             "character",
		Enabled:          &enabled,
		BriefDescription: "港城巡夜司执事。",
	}); err != nil {
		t.Fatal(err)
	}

	server := NewServer(application, "0")
	beforeHistoryResp := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	if beforeHistoryResp.Code != http.StatusOK {
		t.Fatalf("read chat history before inline edit: status=%d body=%s", beforeHistoryResp.Code, beforeHistoryResp.Body.String())
	}

	reqBytes, _ := json.Marshal(map[string]any{
		"mode":        "writing",
		"instruction": "增强氛围",
		"selection":   "温照野说：“潮汐快涨了。”",
		"before":      "港城夜雾弥漫。",
		"lore_ids":    []string{"wen-zhaoye"},
	})
	rec := ut.PerformRequest(
		server.engine.Engine,
		"POST",
		"/api/projects/"+projectID+"/inline-edit/stream",
		&ut.Body{Body: bytes.NewReader(reqBytes), Len: len(reqBytes)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "Accept-Encoding", Value: "gzip"},
	)

	if rec.Code != consts.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if enc := string(rec.Header().Peek("Content-Encoding")); enc != "" {
		t.Fatalf("SSE stream must bypass gzip, got Content-Encoding=%q", enc)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"event: meta\n",
		`"lore_count":1`,
		"event: delta\n",
		"event: done\n",
		"夜风掠过栈桥，温照野低声道：“潮汐将至。”",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("SSE response missing %q:\n%s", needle, body)
		}
	}

	afterHistoryResp := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	if afterHistoryResp.Code != http.StatusOK {
		t.Fatalf("read chat history after inline edit: status=%d", afterHistoryResp.Code)
	}
	if afterHistoryResp.Body.String() != beforeHistoryResp.Body.String() {
		t.Fatalf("inline edit mutated active session history:\nbefore=%s\nafter=%s", beforeHistoryResp.Body.String(), afterHistoryResp.Body.String())
	}
}

func TestInlineEditRejectsUnconfiguredDirectModel(t *testing.T) {
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "gpt-5",
		OpenAIBaseURL:       "https://api.openai.com/v1",
		OpenAIAPIKey:        "",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	server := NewServer(application, "0")
	rec := performJSONRequest(t, server, "POST", "/api/projects/"+application.ProjectID()+"/inline-edit/stream", map[string]any{
		"mode":        "writing",
		"instruction": "润色",
		"selection":   "这是一段测试正文。",
	})
	if rec.Code != consts.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	decodeResponse(t, rec.Body.Bytes(), &payload)
	if payload["code"] != "api.inlineEdit.modelRequired" {
		t.Fatalf("error code = %v, want api.inlineEdit.modelRequired", payload["code"])
	}
}
