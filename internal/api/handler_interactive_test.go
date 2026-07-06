package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agent"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveStoriesAndTellersAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list stories status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var initial struct {
		CurrentStoryID string `json:"current_story_id"`
		Stories        []any  `json:"stories"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &initial)
	if initial.CurrentStoryID != "" || len(initial.Stories) != 0 {
		t.Fatalf("initial stories should be empty: %#v", initial)
	}

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "末日开端",
		"origin":          "主角醒来发现世界已末日",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		StoryTellerID string `json:"story_teller_id"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.Title != "末日开端" || created.StoryTellerID != "classic" {
		t.Fatalf("created story mismatch: %#v", created)
	}

	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot struct {
		StoryID            string                          `json:"story_id"`
		BranchID           string                          `json:"branch_id"`
		Turns              []any                           `json:"turns"`
		DirectorPlanStatus *interactive.DirectorPlanStatus `json:"director_plan_status"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if snapshot.StoryID != created.ID || snapshot.BranchID != "main" || len(snapshot.Turns) != 0 {
		t.Fatalf("snapshot mismatch: %#v", snapshot)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusWaitingOpening || snapshot.DirectorPlanStatus.Blocking {
		t.Fatalf("new story snapshot should expose waiting director status without blocking: %#v", snapshot.DirectorPlanStatus)
	}
	var rawSnapshot map[string]json.RawMessage
	decodeResponse(t, snapshotResp.Body.Bytes(), &rawSnapshot)
	if _, ok := rawSnapshot["director_plan"]; ok {
		t.Fatalf("snapshot must not expose full director plan docs: %s", snapshotResp.Body.String())
	}

	if _, err := application.AppendInteractiveTurn(created.ID, "", "我推开酒馆的门", "门后传来低沉的风声。"); err != nil {
		t.Fatal(err)
	}
	memoryCreateResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/memory", map[string]any{
		"branch_id":  "main",
		"title":      "酒馆风声",
		"summary":    "门后传来低沉风声。",
		"people":     []string{"主角"},
		"places":     []string{"酒馆"},
		"tags":       []string{"线索"},
		"importance": 4,
	})
	if memoryCreateResp.Code != http.StatusOK {
		t.Fatalf("create memory status = %d body=%s", memoryCreateResp.Code, memoryCreateResp.Body.String())
	}
	var memoryEntry struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Manual bool   `json:"manual"`
	}
	decodeResponse(t, memoryCreateResp.Body.Bytes(), &memoryEntry)
	if memoryEntry.ID == "" || memoryEntry.Title != "酒馆风声" || !memoryEntry.Manual {
		t.Fatalf("memory entry mismatch: %#v", memoryEntry)
	}
	memoryListResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/memory?branch=main", nil)
	if memoryListResp.Code != http.StatusOK {
		t.Fatalf("list memory status = %d body=%s", memoryListResp.Code, memoryListResp.Body.String())
	}
	var memoryList struct {
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	decodeResponse(t, memoryListResp.Body.Bytes(), &memoryList)
	if len(memoryList.Entries) != 1 || memoryList.Entries[0].ID != memoryEntry.ID {
		t.Fatalf("memory list mismatch: %#v", memoryList)
	}
	archiveResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/memory/"+memoryEntry.ID+"/archive", map[string]bool{"archived": true})
	if archiveResp.Code != http.StatusOK {
		t.Fatalf("archive memory status = %d body=%s", archiveResp.Code, archiveResp.Body.String())
	}
	snapshotResp = performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if len(snapshot.Turns) != 1 {
		t.Fatalf("chat should persist one turn: %#v", snapshot)
	}

	branchResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/branches", map[string]string{
		"parent_event_id": snapshot.Turns[0].(map[string]any)["id"].(string),
		"title":           "换条路走",
	})
	if branchResp.Code != http.StatusOK {
		t.Fatalf("branch status = %d body=%s", branchResp.Code, branchResp.Body.String())
	}
	var branch struct {
		ID string `json:"id"`
	}
	decodeResponse(t, branchResp.Body.Bytes(), &branch)
	if branch.ID == "" {
		t.Fatalf("branch id should not be empty: %#v", branch)
	}

	patchResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID, map[string]string{
		"title":           "新标题",
		"story_teller_id": "grimdark",
	})
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}

	deleteResp := performJSONRequest(t, server, http.MethodDelete, "/api/interactive/stories/"+created.ID, nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}

	tellersResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/tellers", nil)
	if tellersResp.Code != http.StatusOK {
		t.Fatalf("list tellers status = %d body=%s", tellersResp.Code, tellersResp.Body.String())
	}
	var tellersBody struct {
		Tellers []struct {
			ID string `json:"id"`
		} `json:"tellers"`
	}
	decodeResponse(t, tellersResp.Body.Bytes(), &tellersBody)
	if len(tellersBody.Tellers) < 3 {
		t.Fatalf("expected built-in tellers: %#v", tellersBody.Tellers)
	}

	classicResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/tellers/classic", nil)
	if classicResp.Code != http.StatusOK {
		t.Fatalf("get teller status = %d body=%s", classicResp.Code, classicResp.Body.String())
	}
	var classic struct {
		ID    string `json:"id"`
		Slots []struct {
			ID      string `json:"id"`
			Target  string `json:"target"`
			Content string `json:"content"`
		} `json:"slots"`
	}
	decodeResponse(t, classicResp.Body.Bytes(), &classic)
	if classic.ID != "classic" || len(classic.Slots) == 0 || classic.Slots[0].Content == "" {
		t.Fatalf("classic teller mismatch: %#v", classic)
	}
}

func TestInteractiveDirectorAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "导演接口",
		"origin":          "主角准备参加学院大比",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)

	statusResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/director/status", nil)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("get director status endpoint = %d body=%s", statusResp.Code, statusResp.Body.String())
	}
	var status interactive.DirectorPlanStatus
	decodeResponse(t, statusResp.Body.Bytes(), &status)
	if status.Status != interactive.DirectorPlanStatusWaitingOpening || status.Blocking || status.StartReady || status.CompletedDocs != 0 || status.PlannedDocs != 1 {
		t.Fatalf("initial director status mismatch: %#v", status)
	}

	getResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/director", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get director status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	type directorResponse struct {
		Docs struct {
			Plan string `json:"plan"`
		} `json:"docs"`
		Metadata struct {
			Revision string `json:"revision"`
			LastRun  struct {
				Status string `json:"status"`
			} `json:"last_run"`
		} `json:"metadata"`
	}
	var director directorResponse
	decodeResponse(t, getResp.Body.Bytes(), &director)
	if director.Metadata.LastRun.Status != interactive.DirectorPlanStatusWaitingOpening || !strings.Contains(director.Docs.Plan, "正文Agent可读") {
		t.Fatalf("default director plan mismatch: %#v", director)
	}

	nextDocs := director.Docs
	nextDocs.Plan += "\n\n手动设置主线：学院逆袭主线。"
	patchResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID+"/director", map[string]any{
		"docs":          nextDocs,
		"base_revision": director.Metadata.Revision,
		"summary":       "手动设置主线",
	})
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch director status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}
	decodeResponse(t, patchResp.Body.Bytes(), &director)
	if !strings.Contains(director.Docs.Plan, "学院逆袭主线") || director.Metadata.LastRun.Status != "ready" {
		t.Fatalf("director plan patch mismatch: %#v", director)
	}

	rebuildResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/rebuild", nil)
	if rebuildResp.Code != http.StatusOK {
		t.Fatalf("rebuild director status = %d body=%s", rebuildResp.Code, rebuildResp.Body.String())
	}
	director = directorResponse{}
	decodeResponse(t, rebuildResp.Body.Bytes(), &director)
	if !strings.Contains(director.Docs.Plan, "正文Agent可读") || director.Metadata.LastRun.Status != "ready" {
		t.Fatalf("rebuilt director plan mismatch: %#v", director)
	}

	if _, err := application.AppendInteractiveTurn(created.ID, "", "我报名学院大比", "报名弟子把他的名字写进木牌。"); err != nil {
		t.Fatal(err)
	}
	if _, err := application.InteractiveDirectorPlanStatus(created.ID, "main"); err != nil {
		t.Fatal(err)
	}
	runResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/run", map[string]string{"branch_id": "main"})
	if runResp.Code != http.StatusOK {
		t.Fatalf("run director status = %d body=%s", runResp.Code, runResp.Body.String())
	}
	status = waitForDirectorStatusAPI(t, server, created.ID, interactive.DirectorPlanStatusReady)
	if !status.StartReady || status.Blocking || status.CompletedDocs != status.PlannedDocs {
		t.Fatalf("manual director run should become ready: %#v", status)
	}
}

func TestInteractiveStoryCreateDoesNotRunInitialDirector(t *testing.T) {
	application := newTestApplication(t)
	calls := 0
	restoreDirector := runtimeapp.SetInteractiveDirectorGeneratorForTest(func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		calls++
		return "", errors.New("director unavailable")
	})
	t.Cleanup(restoreDirector)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "失败回滚",
		"origin":          "主角准备出发",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story should not run initial director status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	if calls != 0 {
		t.Fatalf("director generator should not run during story creation, calls=%d", calls)
	}

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list stories status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var list struct {
		Stories []struct {
			ID string `json:"id"`
		} `json:"stories"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &list)
	if len(list.Stories) != 1 || list.Stories[0].ID == "" {
		t.Fatalf("story should be committed after creation without director run: %#v", list)
	}
	statusResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+list.Stories[0].ID+"/director/status", nil)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("director status after create = %d body=%s", statusResp.Code, statusResp.Body.String())
	}
	var status interactive.DirectorPlanStatus
	decodeResponse(t, statusResp.Body.Bytes(), &status)
	if status.Status != interactive.DirectorPlanStatusWaitingOpening || status.Blocking {
		t.Fatalf("created story should wait for opening before director run: %#v", status)
	}
}

func TestInteractiveOpeningRollAndInitialStateAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	rollResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/opening/roll", map[string]any{
		"story_director_id": "default",
		"seed":              42,
	})
	if rollResp.Code != http.StatusOK {
		t.Fatalf("opening roll status = %d body=%s", rollResp.Code, rollResp.Body.String())
	}
	var rolled struct {
		StoryDirectorID string `json:"story_director_id"`
		Seed            int64  `json:"seed"`
		StateOps        []any  `json:"state_ops"`
	}
	decodeResponse(t, rollResp.Body.Bytes(), &rolled)
	if rolled.StoryDirectorID != "default" || rolled.Seed != 42 || len(rolled.StateOps) == 0 {
		t.Fatalf("opening roll mismatch: %#v", rolled)
	}

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]any{
		"title":           "带开局状态",
		"story_teller_id": "classic",
		"initial_state_ops": []map[string]any{{
			"op":    "set",
			"path":  "resources.hp",
			"value": 18,
		}},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story with initial state status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Events int    `json:"events"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.Events != 1 {
		t.Fatalf("created story with initial state mismatch: %#v", created)
	}
	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot struct {
		State map[string]any `json:"state"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	actors, _ := snapshot.State["actors"].(map[string]any)
	protagonist, _ := actors["protagonist"].(map[string]any)
	actorState, _ := protagonist["state"].(map[string]any)
	resources, _ := actorState["resources"].(map[string]any)
	if resources["hp"] != float64(18) {
		t.Fatalf("initial state should be visible in snapshot: %#v", snapshot.State)
	}
}

func TestInteractiveDisabledStoryDirectorModulesAPI(t *testing.T) {
	application := newTestApplication(t)
	if _, err := application.CreateStoryDirector(interactive.StoryDirector{
		ID:   "detached",
		Name: "关闭模块导演",
		ModuleRefs: interactive.StoryDirectorModuleRefs{
			NarrativeStyleID:        "non-classic-style",
			NarrativeStyleDisabled:  true,
			EventSystemID:           "default",
			EventSystemDisabled:     true,
			RuleSystemID:            "default",
			RuleSystemDisabled:      true,
			OpeningSelectorID:       "default",
			OpeningSelectorDisabled: true,
			ImagePresetID:           "non-default-image",
			ImagePresetDisabled:     true,
		},
		Strategy: interactive.StoryDirectorStrategy{Enabled: true},
	}); err != nil {
		t.Fatalf("create detached story director failed: %v", err)
	}
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]any{
		"title":             "关闭模块故事",
		"story_director_id": "detached",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create detached story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID              string `json:"id"`
		StoryTellerID   string `json:"story_teller_id"`
		StoryDirectorID string `json:"story_director_id"`
		ImageSettings   struct {
			PresetID string `json:"preset_id"`
		} `json:"image_settings"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.StoryDirectorID != "detached" {
		t.Fatalf("created detached story mismatch: %#v", created)
	}
	if created.StoryTellerID != "classic" {
		t.Fatalf("disabled narrative style should not be inherited, got %#v", created)
	}
	if created.ImageSettings.PresetID != "game-cg" {
		t.Fatalf("disabled image preset should not be inherited, got %#v", created.ImageSettings)
	}

	rebuildResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/rebuild", nil)
	if rebuildResp.Code != http.StatusOK {
		t.Fatalf("rebuild detached director status = %d body=%s", rebuildResp.Code, rebuildResp.Body.String())
	}
	var rebuilt struct {
		Docs struct {
			Plan string `json:"plan"`
		} `json:"docs"`
	}
	decodeResponse(t, rebuildResp.Body.Bytes(), &rebuilt)
	if !strings.Contains(rebuilt.Docs.Plan, "正文Agent可读") {
		t.Fatalf("rebuilt detached director should return plan docs: %#v", rebuilt)
	}
}

func TestStoryMemoryStructuresPresetAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list story memory structures status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var list struct {
		Items []interactive.StoryMemoryStructureModule `json:"story_memory_structures"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &list)
	if len(list.Items) == 0 || list.Items[0].ID == "" {
		t.Fatalf("list should include built-in memory structure modules: %#v", list)
	}

	enabled := true
	createResp := performJSONRequest(t, server, http.MethodPost, "/api/story-memory-structures", interactive.StoryMemoryStructureModule{
		ID:   "custom-memory-api",
		Name: "API 记忆结构",
		Structures: []interactive.StoryMemoryStructure{{
			ID:      "quest",
			Name:    "任务",
			Mode:    "keyed",
			Enabled: &enabled,
			Fields: []interactive.StoryMemoryField{
				{ID: "name", Name: "名称", Required: true, Order: 10},
				{ID: "status", Name: "状态", Order: 20},
			},
			KeyFieldID: "name",
			Order:      10,
		}},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story memory structure status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created interactive.StoryMemoryStructureModule
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID != "custom-memory-api" || !created.Custom || len(created.Structures) != 1 {
		t.Fatalf("created story memory structure mismatch: %#v", created)
	}

	getResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures/custom-memory-api", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get story memory structure status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	var loaded interactive.StoryMemoryStructureModule
	decodeResponse(t, getResp.Body.Bytes(), &loaded)
	if loaded.ID != created.ID || loaded.UpdatedAt != created.UpdatedAt {
		t.Fatalf("loaded story memory structure mismatch: %#v", loaded)
	}

	conflictResp := performJSONRequest(t, server, http.MethodPatch, "/api/story-memory-structures/custom-memory-api", map[string]any{
		"id":            "custom-memory-api",
		"name":          "冲突名称",
		"structures":    created.Structures,
		"base_revision": "stale-revision",
	})
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("stale story memory structure update should conflict, status = %d body=%s", conflictResp.Code, conflictResp.Body.String())
	}

	updateResp := performJSONRequest(t, server, http.MethodPatch, "/api/story-memory-structures/custom-memory-api", map[string]any{
		"id":            "custom-memory-api",
		"name":          "更新后的 API 记忆结构",
		"structures":    created.Structures,
		"base_revision": created.UpdatedAt,
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update story memory structure status = %d body=%s", updateResp.Code, updateResp.Body.String())
	}
	var updated interactive.StoryMemoryStructureModule
	decodeResponse(t, updateResp.Body.Bytes(), &updated)
	if updated.Name != "更新后的 API 记忆结构" || updated.UpdatedAt == created.UpdatedAt {
		t.Fatalf("updated story memory structure mismatch: %#v", updated)
	}

	deleteResp := performJSONRequest(t, server, http.MethodDelete, "/api/story-memory-structures/custom-memory-api", nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete story memory structure status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	missingResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures/custom-memory-api", nil)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("deleted story memory structure should be missing, status = %d body=%s", missingResp.Code, missingResp.Body.String())
	}
}

func TestInteractiveChatRequiresStoryID(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	resp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat", map[string]string{
		"mode":    "story",
		"message": "我推开酒馆的门",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("chat status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "故事 ID 不能为空") {
		t.Fatalf("unexpected response body: %s", resp.Body.String())
	}
}

func waitForDirectorStatusAPI(t *testing.T, server *Server, storyID, status string) interactive.DirectorPlanStatus {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	var current interactive.DirectorPlanStatus
	for {
		resp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+storyID+"/director/status?branch=main", nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("director status polling = %d body=%s", resp.Code, resp.Body.String())
		}
		decodeResponse(t, resp.Body.Bytes(), &current)
		if current.Status == status {
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("director status did not reach %q: %#v", status, current)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
