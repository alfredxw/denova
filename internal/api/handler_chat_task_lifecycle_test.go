package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	runtimeapp "denova/internal/app"
	"denova/internal/taskcenter"
)

func TestAgentRunRemainsBoundToItsSessionUntilExplicitlyStoppedAPI(t *testing.T) {
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

	initialSessions := performJSONRequest(t, server, http.MethodGet, "/api/sessions", nil)
	var initial struct {
		Sessions []testSessionDTO `json:"sessions"`
	}
	decodeResponse(t, initialSessions.Body.Bytes(), &initial)
	if len(initial.Sessions) != 1 {
		t.Fatalf("initial sessions = %#v", initial.Sessions)
	}
	sourceSessionID := initial.Sessions[0].ID

	requestBody, err := json.Marshal(map[string]string{"message": "keep running"})
	if err != nil {
		t.Fatal(err)
	}
	chatResponse := make(chan *ut.ResponseRecorder, 1)
	go func() {
		chatResponse <- ut.PerformRequest(
			server.engine.Engine,
			http.MethodPost,
			"/api/chat",
			&ut.Body{Body: bytes.NewReader(requestBody), Len: len(requestBody)},
			ut.Header{Key: "Content-Type", Value: "application/json"},
		)
	}()
	<-modelStarted

	createSession := performJSONRequest(t, server, http.MethodPost, "/api/sessions", map[string]string{"title": "other work"})
	if createSession.Code != http.StatusOK {
		t.Fatalf("create session status = %d body=%s", createSession.Code, createSession.Body.String())
	}
	var otherSession testSessionDTO
	decodeResponse(t, createSession.Body.Bytes(), &otherSession)

	sourceActive := performJSONRequest(t, server, http.MethodGet, "/api/chat/active?session_id="+url.QueryEscape(sourceSessionID), nil)
	var sourceStatus struct {
		Active    bool   `json:"active"`
		Status    string `json:"status"`
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
	}
	decodeResponse(t, sourceActive.Body.Bytes(), &sourceStatus)
	if !sourceStatus.Active || sourceStatus.Status != string(runtimeapp.TaskRunning) || sourceStatus.TaskID == "" || sourceStatus.SessionID != sourceSessionID {
		t.Fatalf("source session should retain its running task: %#v", sourceStatus)
	}

	otherActive := performJSONRequest(t, server, http.MethodGet, "/api/chat/active?session_id="+url.QueryEscape(otherSession.ID), nil)
	var otherStatus struct {
		Active bool `json:"active"`
	}
	decodeResponse(t, otherActive.Body.Bytes(), &otherStatus)
	if otherStatus.Active {
		t.Fatalf("new session must not inherit another session's task: %#v", otherStatus)
	}

	tasksResponse := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResponse.Body.Bytes(), &tasks)
	var sourceTask *taskcenter.Task
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Type == taskcenter.TaskTypeAgent && tasks.Tasks[i].Recovery.SessionID == sourceSessionID {
			sourceTask = &tasks.Tasks[i]
			break
		}
	}
	if sourceTask == nil || sourceTask.Status != taskcenter.StatusRunning || sourceTask.Recovery.Workspace == "" {
		t.Fatalf("task center should expose the source Agent run: %#v", tasks.Tasks)
	}

	abortResponse := performJSONRequest(t, server, http.MethodPost, "/api/chat/abort?session_id="+url.QueryEscape(sourceSessionID), nil)
	if abortResponse.Code != http.StatusOK {
		t.Fatalf("abort status = %d body=%s", abortResponse.Code, abortResponse.Body.String())
	}
	releaseOnce.Do(func() { close(releaseModel) })
	response := <-chatResponse
	if response.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%s", response.Code, response.Body.String())
	}

	stoppedResponse := performJSONRequest(t, server, http.MethodGet, "/api/chat/active?session_id="+url.QueryEscape(sourceSessionID), nil)
	decodeResponse(t, stoppedResponse.Body.Bytes(), &sourceStatus)
	if sourceStatus.Active || sourceStatus.Status != string(runtimeapp.TaskAborted) {
		t.Fatalf("explicitly stopped source task = %#v", sourceStatus)
	}
}

func TestInteractiveRunsRemainBoundToTheirStoriesAndAppearInTaskCenterAPI(t *testing.T) {
	modelRequests := make(chan struct{}, 2)
	releaseModel := make(chan struct{})
	var releaseOnce sync.Once
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelRequests <- struct{}{}
		select {
		case <-releaseModel:
			http.Error(w, "test model released", http.StatusServiceUnavailable)
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
		application.AbortInteractiveTask()
		releaseOnce.Do(func() { close(releaseModel) })
		application.Close()
	})
	server := NewServer(application, "0")

	createStory := func(title string) string {
		response := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
			"title":           title,
			"story_teller_id": "classic",
		})
		if response.Code != http.StatusOK {
			t.Fatalf("create story %q status = %d body=%s", title, response.Code, response.Body.String())
		}
		var story struct {
			ID string `json:"id"`
		}
		decodeResponse(t, response.Body.Bytes(), &story)
		return story.ID
	}
	storyA := createStory("Story A")
	storyB := createStory("Story B")

	startStoryRun := func(storyID, message string) <-chan *ut.ResponseRecorder {
		body, marshalErr := json.Marshal(map[string]string{
			"mode":     "story",
			"story_id": storyID,
			"branch":   "main",
			"message":  message,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		response := make(chan *ut.ResponseRecorder, 1)
		go func() {
			response <- ut.PerformRequest(
				server.engine.Engine,
				http.MethodPost,
				"/api/interactive/chat",
				&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
				ut.Header{Key: "Content-Type", Value: "application/json"},
			)
		}()
		return response
	}
	responseA := startStoryRun(storyA, "open door A")
	<-modelRequests
	responseB := startStoryRun(storyB, "open door B")
	<-modelRequests

	for _, storyID := range []string{storyA, storyB} {
		activeResponse := performJSONRequest(t, server, http.MethodGet, "/api/interactive/chat/active?story_id="+url.QueryEscape(storyID)+"&branch=main", nil)
		var active struct {
			Active   bool   `json:"active"`
			Status   string `json:"status"`
			TaskID   string `json:"task_id"`
			StoryID  string `json:"story_id"`
			BranchID string `json:"branch_id"`
		}
		decodeResponse(t, activeResponse.Body.Bytes(), &active)
		if !active.Active || active.Status != string(runtimeapp.TaskRunning) || active.TaskID == "" || active.StoryID != storyID || active.BranchID != "main" {
			t.Fatalf("story %s should retain its running task: %#v", storyID, active)
		}
	}

	tasksResponse := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResponse.Body.Bytes(), &tasks)
	interactiveTasks := make(map[string]taskcenter.Task)
	for _, task := range tasks.Tasks {
		if task.Type == taskcenter.TaskTypeInteractiveStory {
			interactiveTasks[task.Recovery.StoryID] = task
		}
	}
	for _, storyID := range []string{storyA, storyB} {
		task, ok := interactiveTasks[storyID]
		if !ok || task.Status != taskcenter.StatusRunning || task.Recovery.BranchID != "main" || task.Recovery.Workspace == "" {
			t.Fatalf("task center missing story run %s: %#v", storyID, tasks.Tasks)
		}
	}

	abortA := performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat/abort?story_id="+url.QueryEscape(storyA)+"&branch=main", nil)
	if abortA.Code != http.StatusOK {
		t.Fatalf("abort story A status = %d body=%s", abortA.Code, abortA.Body.String())
	}
	stoppedA := performJSONRequest(t, server, http.MethodGet, "/api/interactive/chat/active?story_id="+url.QueryEscape(storyA)+"&branch=main", nil)
	var stopped struct {
		Active bool   `json:"active"`
		Status string `json:"status"`
	}
	decodeResponse(t, stoppedA.Body.Bytes(), &stopped)
	if stopped.Active || stopped.Status != string(runtimeapp.TaskAborted) {
		t.Fatalf("explicitly stopped story A task = %#v", stopped)
	}

	_ = performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat/abort?story_id="+url.QueryEscape(storyB)+"&branch=main", nil)
	<-responseA
	<-responseB
}
