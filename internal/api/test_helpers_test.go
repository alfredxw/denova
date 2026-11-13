package api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	runtimeapp "denova/internal/app"
)

func newTestApplication(t *testing.T) *runtimeapp.App {
	t.Helper()
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "http://127.0.0.1:1/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return application
}

func activeWritingSessionID(t *testing.T, application *runtimeapp.App) string {
	t.Helper()
	sessions, err := application.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if session.Active {
			return session.ID
		}
	}
	t.Fatal("test application has no active Writing Session")
	return ""
}

func performJSONRequest(t *testing.T, server *Server, method, path string, body any) *ut.ResponseRecorder {
	t.Helper()
	var requestBody *ut.Body
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = &ut.Body{Body: bytes.NewReader(data), Len: len(data)}
	}
	return ut.PerformRequest(server.engine.Engine, method, path, requestBody, ut.Header{Key: "Content-Type", Value: "application/json"})
}

func decodeResponse(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode response: %v body=%s", err, string(data))
	}
}
