package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

type sceneTestHost struct {
	StoryHost
	scope            Scope
	turnID, revision string
	calls            int
}

func (h *sceneTestHost) Scene(_ context.Context, scope Scope, turnID, revision string) (StoryScene, error) {
	h.scope, h.turnID, h.revision = scope, turnID, revision
	h.calls++
	return StoryScene{Turn: StoryTurn{ID: turnID, Revision: revision}, State: StoryState{StoryID: scope.StoryID, BranchID: scope.BranchID, TurnID: turnID, SourceRevision: revision}}, nil
}

func TestStorySceneRequiresOwningReaderAndExactAddress(t *testing.T) {
	host := &sceneTestHost{}
	caller := &activation{grants: []string{"stories.read"}, context: RuntimeContext{Scope: Scope{Kind: "game-instance", ProjectID: "project", StoryID: "story", InstanceID: "instance"}}}
	runtime := &Runtime{manager: &Manager{stories: host}, owner: caller}
	call := func(query string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		runtime.serveStory(response, httptest.NewRequest("GET", "http://localhost/story/scene?"+query, nil), caller, "/story/scene")
		return response
	}
	valid := "branchId=main&turnId=old&sourceRevision=revision"
	response := call(valid + "&storyId=other&projectId=other")
	if response.Code != 200 || host.scope.StoryID != "story" || host.scope.ProjectID != "project" || host.scope.BranchID != "main" || host.turnID != "old" || host.revision != "revision" {
		t.Fatalf("scene scope was widened: %+v %s", host, response.Body.String())
	}
	var scene StoryScene
	if err := json.Unmarshal(response.Body.Bytes(), &scene); err != nil {
		t.Fatal(err)
	}
	if scene.State.SourceRevision != scene.Turn.Revision {
		t.Fatal("scene revision mismatch")
	}
	for _, query := range []string{"", "branchId=main&turnId=old", valid + "&turnId=other", "branchId=%20main&turnId=old&sourceRevision=revision"} {
		if response := call(query); response.Code != 400 {
			t.Fatalf("ambiguous address accepted: %q %s", query, response.Body.String())
		}
	}
	for _, mutate := range []func(){func() { runtime.owner = &activation{} }, func() { runtime.owner = caller; runtime.hostOnly = true }, func() { runtime.hostOnly = false; caller.grants = []string{"stories.write"} }} {
		mutate()
		if response := call(valid); response.Code != 403 {
			t.Fatalf("unauthorized scene read: %s", response.Body.String())
		}
	}
	if host.calls != 1 {
		t.Fatalf("rejected request reached host: %d", host.calls)
	}
	operation := OpenAPI()["paths"].(map[string]any)["/story/scene"].(map[string]any)["get"].(map[string]any)
	for _, parameter := range operation["parameters"].([]map[string]any) {
		if parameter["required"] != true {
			t.Fatalf("scene address is optional in discovery: %+v", parameter)
		}
	}
}
