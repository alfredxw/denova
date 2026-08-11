package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	runtimeapp "denova/internal/app"
	"denova/internal/taskcenter"
)

func TestConfigManagerTaskAppearsInTaskCenterAPI(t *testing.T) {
	modelStarted := make(chan struct{})
	releaseModel := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(modelStarted) })
		select {
		case <-releaseModel:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(modelServer.Close)

	workspace := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIAPIKey:        "test-key",
		OpenAIBaseURL:       modelServer.URL + "/v1",
		OpenAIModel:         "test-model",
		NovaDir:             workspace,
		Workspace:           workspace,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseModel) })
		application.Close()
	})
	server := NewServer(application, "0")

	requestBody, err := json.Marshal(map[string]string{
		"instruction": "调整写作模式方案预设",
		"origin":      "agents",
		"resource_id": "agents:main",
	})
	if err != nil {
		t.Fatal(err)
	}
	streamResponse := make(chan *ut.ResponseRecorder, 1)
	go func() {
		streamResponse <- ut.PerformRequest(
			server.engine.Engine,
			http.MethodPost,
			"/api/config-manager/stream",
			&ut.Body{Body: bytes.NewReader(requestBody), Len: len(requestBody)},
			ut.Header{Key: "Content-Type", Value: "application/json"},
		)
	}()
	<-modelStarted

	tasksResp := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if tasksResp.Code != http.StatusOK {
		t.Fatalf("task center status = %d body=%s", tasksResp.Code, tasksResp.Body.String())
	}
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResp.Body.Bytes(), &tasks)
	var configTask *taskcenter.Task
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Type == taskcenter.TaskTypeAgent && tasks.Tasks[i].Recovery.Kind == taskcenter.RecoveryConfigManager {
			configTask = &tasks.Tasks[i]
			break
		}
	}
	if configTask == nil {
		t.Fatalf("task center missing config-manager task: %#v", tasks.Tasks)
	}
	if configTask.Status != taskcenter.StatusRunning {
		t.Fatalf("config-manager task status = %q, want %q", configTask.Status, taskcenter.StatusRunning)
	}
	if configTask.Project.Path != workspace || configTask.Recovery.Workspace != workspace {
		t.Fatalf("config-manager task project = %#v recovery=%#v", configTask.Project, configTask.Recovery)
	}
	if configTask.Recovery.Origin != "agents" || configTask.Recovery.ResourceID != "agents:main" {
		t.Fatalf("config-manager recovery scope = %#v", configTask.Recovery)
	}
	if configTask.Recovery.TaskID == "" {
		t.Fatalf("config-manager task is missing its execution task ID: %#v", configTask)
	}
	if !strings.Contains(configTask.Title, "调整写作模式方案预设") {
		t.Fatalf("config-manager task title = %q, want instruction-derived title", configTask.Title)
	}

	releaseOnce.Do(func() { close(releaseModel) })
	<-streamResponse
}
