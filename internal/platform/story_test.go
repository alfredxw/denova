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

type stateTestHost struct {
	StoryHost
	scope         Scope
	turnID        string
	configuration StoryConfiguration
	imported      StoryPresetImport
}

func (h *stateTestHost) ImportPreset(_ context.Context, scope Scope, input StoryPresetImport) (StoryPreset, error) {
	h.scope, h.imported = scope, input
	return StoryPreset{Kind: input.Kind, ID: "host-generated", Name: input.Name}, nil
}

func TestStoryPresetImportRequiresOwningBoundWriter(t *testing.T) {
	host := &stateTestHost{}
	caller := &activation{grants: []string{"stories.read", "stories.write"}, context: RuntimeContext{Scope: Scope{Kind: "game-instance", ProjectID: "project", StoryID: "story", InstanceID: "instance"}}}
	runtime := &Runtime{manager: &Manager{stories: host}, owner: caller}
	call := func(body string) int {
		response := httptest.NewRecorder()
		runtime.serveStory(response, httptest.NewRequest("POST", "http://localhost/story/presets/import", strings.NewReader(body)), caller, "/story/presets/import")
		return response.Code
	}
	valid := `{"kind":"planning","name":"Work plan","description":"","content":[]}`
	if code := call(valid); code != 200 || host.scope.ProjectID != "project" || host.imported.Name != "Work plan" {
		t.Fatalf("bound import failed: %d %+v", code, host)
	}
	if code := call(`{"kind":"planning","name":"Plan","content":[],"id":"local-id"}`); code != 400 {
		t.Fatalf("author ID accepted: %d", code)
	}
	runtime.owner = &activation{}
	if code := call(valid); code != 403 {
		t.Fatalf("dependency imported shared preset: %d", code)
	}
	runtime.owner = caller
	caller.grants = []string{"stories.read"}
	if code := call(valid); code != 403 {
		t.Fatalf("read grant imported preset: %d", code)
	}
}

func (h *stateTestHost) State(_ context.Context, scope Scope, turnID string) (StoryState, error) {
	h.scope, h.turnID = scope, turnID
	return StoryState{StoryID: scope.StoryID, BranchID: scope.BranchID, TurnID: turnID, State: map[string]any{}}, nil
}

func (h *stateTestHost) Configure(_ context.Context, scope Scope, input StoryConfiguration) (StorySnapshot, error) {
	h.scope, h.configuration = scope, input
	return StorySnapshot{StoryID: scope.StoryID}, nil
}

func (h *stateTestHost) Tune(_ context.Context, scope Scope, input StoryPacing) (StorySnapshot, error) {
	h.scope = scope
	h.configuration.ReplyTargetChars = input.ReplyTargetChars
	return StorySnapshot{StoryID: scope.StoryID}, nil
}

func TestStoryPacingUsesBoundWriterAndRejectsStructuralInput(t *testing.T) {
	host := &stateTestHost{}
	runtime := &Runtime{manager: &Manager{stories: host}}
	caller := &activation{grants: []string{"stories.read"}, context: RuntimeContext{Scope: Scope{Kind: "game-instance", ProjectID: "project", StoryID: "story", InstanceID: "instance"}}}
	call := func(body string) int {
		response := httptest.NewRecorder()
		runtime.serveStory(response, httptest.NewRequest("PUT", "http://localhost/story/pacing?storyId=other", strings.NewReader(body)), caller, "/story/pacing")
		return response.Code
	}
	if call(`{"replyTargetChars":2000}`) != 403 {
		t.Fatal("read grant changed pacing")
	}
	caller.grants = append(caller.grants, "stories.write")
	if call(`{"replyTargetChars":2000}`) != 200 || host.scope.StoryID != "story" || host.configuration.ReplyTargetChars != 2000 {
		t.Fatal("pacing escaped bound Story or was not applied")
	}
	if call(`{"replyTargetChars":2000,"origin":"replace"}`) != 400 {
		t.Fatal("pacing accepted structural input")
	}
}

func TestStoryStateAndConfigurationUseBoundAuthority(t *testing.T) {
	host := &stateTestHost{}
	runtime := &Runtime{manager: &Manager{stories: host}}
	scope := Scope{Kind: "game-instance", ProjectID: "project", StoryID: "story", InstanceID: "instance"}
	caller := &activation{grants: []string{"stories.read"}, context: RuntimeContext{Scope: scope}}
	response := httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/state?storyId=other&projectId=other&branchId=branch&turnId=turn", nil), caller, "/story/state")
	if response.Code != 200 || host.scope.StoryID != "story" || host.scope.ProjectID != "project" || host.scope.BranchID != "branch" || host.turnID != "turn" {
		t.Fatalf("invalid historical authority: %+v %s", host.scope, response.Body.String())
	}
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("PUT", "http://localhost/story/configuration", strings.NewReader(`{"origin":"Premise"}`)), caller, "/story/configuration")
	if response.Code != 403 {
		t.Fatal("read permission authorized opening mutation")
	}
	caller.grants = append(caller.grants, "stories.write")
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("PUT", "http://localhost/story/configuration", strings.NewReader(`{"origin":"Premise","initialActors":[{"id":"name","name":"Name","templateId":"important_character"}]}`)), caller, "/story/configuration")
	if response.Code != 200 || host.scope != scope || len(host.configuration.InitialActors) != 1 {
		t.Fatalf("configuration failed: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("PUT", "http://localhost/story/configuration", strings.NewReader(`{"origin":"Premise","projectId":"other"}`)), caller, "/story/configuration")
	if response.Code != 400 {
		t.Fatalf("unknown authority field was accepted: %d", response.Code)
	}
}

func (h *streamTestHost) Stream(_ context.Context, scope Scope, operation string, emit func(StoryStreamEvent) error) error {
	h.scope, h.operation = scope, operation
	for _, event := range []StoryStreamEvent{{Kind: "reset"}, {Kind: "activity", Phase: "thinking"}, {Kind: "delta", Text: "你好\n[lin|smile] Welcome"}, {Kind: "settled"}} {
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
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/events?operationId=operation&includeActivity=true", nil), caller, "/story/events")
	if !strings.Contains(response.Body.String(), `"phase":"thinking"`) {
		t.Fatal("requested activity was not streamed")
	}
	caller.grants = nil
	response = httptest.NewRecorder()
	runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/events?operationId=operation", nil), caller, "/story/events")
	if response.Code != 403 {
		t.Fatalf("Stream bypassed permissions: %d", response.Code)
	}
}
