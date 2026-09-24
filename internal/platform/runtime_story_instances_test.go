package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"denova/internal/interactive"
)

// Use the real native journal for receipts so closing/reopening the Store in
// these tests exercises durable recovery rather than an in-memory mock map.
type creationTestStoryHost struct {
	StoryHost
	store          *interactive.Store
	bindCalls      int
	failAfterBind  bool
	failCompletion bool
	lastOwner      string
	lastScope      Scope
}

func (h *creationTestStoryHost) Bind(_ context.Context, instance Instance, _ StoryBindingOptions) (Instance, error) {
	h.bindCalls++
	story, err := h.store.CreateStory(interactive.CreateStoryRequest{Title: instance.Title})
	if err != nil {
		return Instance{}, err
	}
	instance.StoryID = story.ID
	if h.failAfterBind {
		return Instance{}, errors.New("simulated failure after native Story creation")
	}
	return instance, nil
}

func (h *creationTestStoryHost) ReadRecord(ctx context.Context, scope Scope, owner string, address StoryRecordRequest) (StoryRecord, error) {
	h.lastOwner, h.lastScope = owner, scope
	value, revision, err := h.store.ExtensionRecord(ctx, scope.StoryID, address.BranchID, interactive.ExtensionRecord{Owner: owner, Key: address.Key})
	return StoryRecord{Revision: revision, SchemaVersion: value.SchemaVersion, Value: value.Value}, err
}

func (h *creationTestStoryHost) WriteRecord(ctx context.Context, scope Scope, owner string, address StoryRecordRequest) (StoryRecord, error) {
	if h.failCompletion && address.ExpectedRevision != 0 {
		return StoryRecord{}, errors.New("simulated receipt completion failure")
	}
	value := interactive.ExtensionRecord{Owner: owner, Key: address.Key, SchemaVersion: address.SchemaVersion, Value: address.Value}
	revision, err := h.store.SetExtensionRecord(ctx, scope.StoryID, address.BranchID, address.ExpectedRevision, value)
	return StoryRecord{Revision: revision, SchemaVersion: value.SchemaVersion, Value: value.Value}, err
}

func newCreationTestRuntime(t *testing.T) (*Runtime, *creationTestStoryHost, string) {
	t.Helper()
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.create-story", Game)
	manifest := candidate.Manifest
	manifest.Game.Storage.Kind = "story"
	manifest.Game.Story = &StoryDeclaration{ModelSlot: "writer"}
	manifest.ModelSlots = []ModelSlot{{ID: "writer", Kind: "text", TitleKey: "writer"}}
	manifest.Permissions.Required = []string{"stories.read", "stories.write"}
	candidate.files[Game.manifestFile()], _ = json.Marshal(manifest)
	candidate, err := m.freeze(Game, candidate.files)
	if err != nil {
		t.Fatal(err)
	}
	release := testInstall(t, m, candidate)
	workspace := t.TempDir()
	host := &creationTestStoryHost{store: interactive.NewStore(workspace)}
	t.Cleanup(func() { _ = host.store.Close() })
	story, err := host.store.CreateStory(interactive.CreateStoryRequest{Title: "Authoring session"})
	if err != nil {
		t.Fatal(err)
	}
	m.ConfigureStories(host)
	caller := &activation{release: release, grants: release.Grants, context: RuntimeContext{Scope: Scope{Kind: "game-instance", ProjectID: projectID, StoryID: story.ID, InstanceID: "owner-instance"}, Environment: "installed"}}
	runtime := &Runtime{manager: m, ctx: context.Background(), owner: caller, models: map[string]string{"local:writer": "local-profile"}}
	return runtime, host, workspace
}

func TestRuntimeStoryCreationReplaysCanonicalReceiptAndRejectsDifferentInput(t *testing.T) {
	runtime, host, workspace := newCreationTestRuntime(t)
	input := `{"commandId":"play-one","title":"First play","setup":{"initialValue":7}}`
	response := httptest.NewRecorder()
	runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
	if response.Code != 201 {
		t.Fatalf("create failed: %d %s", response.Code, response.Body.String())
	}
	var created Instance
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.StoryID == "" || created.StoryID == runtime.owner.context.Scope.StoryID || created.ProjectID != runtime.owner.context.Scope.ProjectID || created.ReleaseID != runtime.owner.release.Ref.ReleaseID || created.Models["local:writer"] != "local-profile" || created.Setup["initialValue"] != float64(7) {
		t.Fatalf("incorrect new play binding: %+v", created)
	}
	if host.lastOwner != "denova.platform.creations/test.create-story" {
		t.Fatalf("receipt is extension-writable: %q", host.lastOwner)
	}
	if err := host.store.Close(); err != nil {
		t.Fatal(err)
	}
	host.store = interactive.NewStore(workspace)
	response = httptest.NewRecorder()
	runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
	var recovered Instance
	if err := json.Unmarshal(response.Body.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || recovered.ID != created.ID || host.bindCalls != 1 {
		t.Fatalf("creation repeated after restart: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(strings.Replace(input, "First play", "Another play", 1))), runtime.owner)
	if response.Code != 409 || host.bindCalls != 1 {
		t.Fatalf("conflicting command created another play: %s", response.Body.String())
	}
	if host.lastScope.ProjectID != runtime.owner.context.Scope.ProjectID || host.lastScope.StoryID != runtime.owner.context.Scope.StoryID {
		t.Fatal("creation receipts escaped owner Story")
	}
}

func TestRuntimeStoryCreationNeverRetriesUncertainNativeCreation(t *testing.T) {
	for _, failCompletion := range []bool{false, true} {
		t.Run(fmt.Sprint(failCompletion), func(t *testing.T) {
			runtime, host, workspace := newCreationTestRuntime(t)
			host.failAfterBind, host.failCompletion = !failCompletion, failCompletion
			input := `{"commandId":"uncertain","title":"A play","setup":{}}`
			response := httptest.NewRecorder()
			runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
			if response.Code < 400 || host.bindCalls != 1 {
				t.Fatalf("failure was not reported: %s", response.Body.String())
			}
			if err := host.store.Close(); err != nil {
				t.Fatal(err)
			}
			host.store = interactive.NewStore(workspace)
			host.failAfterBind, host.failCompletion = false, false
			response = httptest.NewRecorder()
			runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
			if response.Code != 409 || host.bindCalls != 1 {
				t.Fatalf("uncertain admission caused duplicate play: %d %s", response.Code, response.Body.String())
			}
			index, err := host.store.Index()
			if err != nil || len(index.Stories) != 2 {
				t.Fatalf("unexpected native Story count: %+v %v", index, err)
			}
		})
	}
}

func TestRuntimeStoryCreationSerializesConcurrentViewSubmissions(t *testing.T) {
	runtime, host, _ := newCreationTestRuntime(t)
	input := `{"commandId":"same-tab-command","title":"One play","setup":{}}`
	responses := make([]*httptest.ResponseRecorder, 2)
	var group sync.WaitGroup
	for index := range responses {
		responses[index] = httptest.NewRecorder()
		group.Go(func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("concurrent creation panicked: %v", recovered)
				}
			}()
			runtime.serveCreateStory(responses[index], httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
		})
	}
	group.Wait()
	if host.bindCalls != 1 || responses[0].Code+responses[1].Code != 401 {
		t.Fatalf("concurrent views created duplicate plays: calls=%d statuses=%d,%d", host.bindCalls, responses[0].Code, responses[1].Code)
	}
}

func TestRuntimeStoryCreationRejectsUnscopedAuthorityAndInvalidInput(t *testing.T) {
	runtime, host, _ := newCreationTestRuntime(t)
	for _, input := range []string{
		`{"commandId":"","title":"Play","setup":{}}`,
		`{"commandId":"one","title":" ","setup":{}}`,
		`{"commandId":"one","title":"Play","setup":{"unknown":true}}`,
		`{"commandId":"one","title":"Play","setup":{},"projectId":"foreign"}`,
		`{"commandId":"one","title":"Play","setup":{},"releaseId":"foreign"}`,
	} {
		response := httptest.NewRecorder()
		runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
		if response.Code != 400 {
			t.Fatalf("invalid creation accepted: %d %s", response.Code, response.Body.String())
		}
	}
	input := `{"commandId":"valid","title":"Play","setup":{}}`
	dependency := *runtime.owner
	response := httptest.NewRecorder()
	runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), &dependency)
	if response.Code != 403 {
		t.Fatal("dependency created a Story")
	}
	runtime.owner.grants = nil
	response = httptest.NewRecorder()
	runtime.serveCreateStory(response, httptest.NewRequest("POST", "/story/instances", strings.NewReader(input)), runtime.owner)
	if response.Code != 403 || host.bindCalls != 0 {
		t.Fatal("creation bypassed write permission")
	}
	addresses, err := host.store.ExtensionRecordAddresses(runtime.owner.context.Scope.StoryID, "denova.platform.creations/test.create-story")
	if err != nil || len(addresses) != 0 {
		t.Fatalf("rejected requests admitted receipts: %+v %v", addresses, err)
	}
}
