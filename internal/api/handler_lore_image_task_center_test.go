package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/taskcenter"
)

func TestLoreImageBatchAppearsInTaskCenterWhileRunning(t *testing.T) {
	firstRequestStarted := make(chan struct{})
	releaseFirstRequest := make(chan struct{})
	var firstRequestOnce sync.Once
	var releaseOnce sync.Once
	releaseFirstImageRequest := func() {
		releaseOnce.Do(func() { close(releaseFirstRequest) })
	}
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		firstRequestOnce.Do(func() {
			close(firstRequestStarted)
			<-releaseFirstRequest
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created":       123,
			"output_format": "png",
			"quality":       "high",
			"size":          "2048x2048",
			"data": []map[string]any{{
				"b64_json":       base64.StdEncoding.EncodeToString(loreImageTestPNGBytes()),
				"revised_prompt": "revised prompt",
			}},
		})
	}))
	defer imageServer.Close()
	defer releaseFirstImageRequest()

	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
		ImageAPIKey:         "test-key",
		ImageAPIBaseURL:     imageServer.URL,
		ImageAPIModel:       "gpt-image-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	server := NewServer(application, "0")

	item, err := application.CreateLoreItem(book.LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", Content: "谨慎。"})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{"item_ids": []string{item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		_ = ut.PerformRequest(
			server.engine.Engine,
			http.MethodPost,
			"/api/lore/images/generate/stream",
			&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
			ut.Header{Key: "Content-Type", Value: "application/json"},
		)
	}()

	select {
	case <-firstRequestStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("image generation request did not start")
	}

	tasksResp := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if tasksResp.Code != http.StatusOK {
		t.Fatalf("task center status = %d body=%s", tasksResp.Code, tasksResp.Body.String())
	}
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResp.Body.Bytes(), &tasks)
	var imageTask *taskcenter.Task
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Type == taskcenter.TaskTypeImageGeneration {
			imageTask = &tasks.Tasks[i]
			break
		}
	}
	if imageTask == nil {
		t.Fatalf("task center missing running image generation task: %#v", tasks.Tasks)
	}
	if imageTask.Status != taskcenter.StatusRunning {
		t.Fatalf("image task status = %q, want %q", imageTask.Status, taskcenter.StatusRunning)
	}
	if imageTask.Project.Path != application.Workspace() {
		t.Fatalf("image task project path = %q, want %q", imageTask.Project.Path, application.Workspace())
	}
	if imageTask.Recovery.Kind != taskcenter.RecoveryKind("image_generation") ||
		imageTask.Recovery.Workspace != application.Workspace() ||
		imageTask.Recovery.TaskID == "" {
		t.Fatalf("image task recovery = %#v", imageTask.Recovery)
	}

	releaseFirstImageRequest()
	select {
	case <-streamDone:
	case <-time.After(10 * time.Second):
		t.Fatal("image batch task did not finish")
	}
}
