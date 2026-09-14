package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStoryAPIRequiresManagedInstanceAuthority(t *testing.T) {
	for _, scope := range []Scope{
		{Kind: "story", ProjectID: "project", StoryID: "story", BranchID: "branch"},
		{Kind: "project", ProjectID: "project", StoryID: "story"},
		{Kind: "game-instance", ProjectID: "project", StoryID: "story"},
		{Kind: "game-instance", ProjectID: "project", InstanceID: "instance"},
	} {
		runtime := &Runtime{manager: &Manager{}}
		caller := &activation{grants: []string{"stories.read", "stories.write"}, context: RuntimeContext{Scope: scope}}
		response := httptest.NewRecorder()
		runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story", nil), caller, "/story")
		var body Error
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != 403 || body.Code != "PERMISSION_DENIED" {
			t.Fatalf("scope widened: %+v response=%d %s", scope, response.Code, response.Body.String())
		}
	}
}

type streamTestHost struct {
	StoryHost
	scope     Scope
	operation string
}

func (h *streamTestHost) Stream(_ context.Context, scope Scope, operation string, emit func(StoryStreamEvent) error) error {
	h.scope, h.operation = scope, operation
	for _, event := range []StoryStreamEvent{{Kind: "reset"}, {Kind: "delta", Text: "你好\n[lin|smile] Welcome"}, {Kind: "settled"}} {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

func TestStoryStreamUsesBoundScopeAndFlushesProvisionalEvents(t *testing.T) {
	host := &streamTestHost{}
	runtime := &Runtime{manager: &Manager{stories: host}}
	scope := Scope{Kind: "game-instance", ProjectID: "project", StoryID: "story", InstanceID: "instance"}
	caller := &activation{grants: []string{"stories.read"}, context: RuntimeContext{Scope: scope}}
	response := httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/events?operationId=operation&storyId=other&projectId=other", nil), caller, "/story/events")
	if host.scope != scope || host.operation != "operation" || !response.Flushed || response.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Invalid stream binding: %+v %q %+v", host.scope, host.operation, response)
	}
	var events []StoryStreamEvent
	for _, line := range strings.Split(response.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event StoryStreamEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 3 || events[1].Text != "你好\n[lin|smile] Welcome" || events[2].Kind != "settled" {
		t.Fatalf("Invalid events: %+v", events)
	}
	caller.grants = nil
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/events?operationId=operation", nil), caller, "/story/events")
	if response.Code != 403 {
		t.Fatalf("Stream bypassed permissions: %d", response.Code)
	}
}
