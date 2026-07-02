package api

import (
	"net/http"
	"strings"
	"testing"
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
		StoryID  string `json:"story_id"`
		BranchID string `json:"branch_id"`
		Turns    []any  `json:"turns"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if snapshot.StoryID != created.ID || snapshot.BranchID != "main" || len(snapshot.Turns) != 0 {
		t.Fatalf("snapshot mismatch: %#v", snapshot)
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

	getResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/director", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get director status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	type directorResponse struct {
		Enabled     bool     `json:"enabled"`
		SpoilerMode string   `json:"spoiler_mode"`
		MainArc     string   `json:"main_arc"`
		Forced      []string `json:"forced_events"`
		Disabled    []string `json:"disabled_events"`
		EventQueue  []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"event_queue"`
	}
	var director directorResponse
	decodeResponse(t, getResp.Body.Bytes(), &director)
	if !director.Enabled || director.SpoilerMode != "layered" {
		t.Fatalf("default director mismatch: %#v", director)
	}

	mainArc := "学院逆袭主线"
	patchResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID+"/director", map[string]any{
		"main_arc": &mainArc,
		"summary":  "手动设置主线",
	})
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch director status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}
	decodeResponse(t, patchResp.Body.Bytes(), &director)
	if director.MainArc != mainArc {
		t.Fatalf("director patch mismatch: %#v", director)
	}

	forceResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/events/contest/force", map[string]string{"reason": "安排比拼"})
	if forceResp.Code != http.StatusOK {
		t.Fatalf("force director event status = %d body=%s", forceResp.Code, forceResp.Body.String())
	}
	director = directorResponse{}
	decodeResponse(t, forceResp.Body.Bytes(), &director)
	if len(director.Forced) != 1 || director.Forced[0] != "contest" || !directorEventStatus(director.EventQueue, "contest", "forced") {
		t.Fatalf("forced director event mismatch: %#v", director)
	}

	disableResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/events/contest/disable", nil)
	if disableResp.Code != http.StatusOK {
		t.Fatalf("disable director event status = %d body=%s", disableResp.Code, disableResp.Body.String())
	}
	director = directorResponse{}
	decodeResponse(t, disableResp.Body.Bytes(), &director)
	if len(director.Disabled) != 1 || director.Disabled[0] != "contest" || len(director.Forced) != 0 {
		t.Fatalf("disabled director event mismatch: %#v", director)
	}

	rebuildResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/rebuild", nil)
	if rebuildResp.Code != http.StatusOK {
		t.Fatalf("rebuild director status = %d body=%s", rebuildResp.Code, rebuildResp.Body.String())
	}
	director = directorResponse{}
	decodeResponse(t, rebuildResp.Body.Bytes(), &director)
	if director.MainArc == "" || len(director.EventQueue) == 0 {
		t.Fatalf("rebuilt director mismatch: %#v", director)
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
		DirectorState   struct {
			Enabled bool `json:"enabled"`
		} `json:"director_state"`
	}
	decodeResponse(t, rollResp.Body.Bytes(), &rolled)
	if rolled.StoryDirectorID != "default" || rolled.Seed != 42 || !rolled.DirectorState.Enabled || len(rolled.StateOps) == 0 {
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
	resources, _ := snapshot.State["resources"].(map[string]any)
	if resources["hp"] != float64(18) {
		t.Fatalf("initial state should be visible in snapshot: %#v", snapshot.State)
	}
}

func directorEventStatus(events []struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}, id, status string) bool {
	for _, event := range events {
		if event.ID == id && event.Status == status {
			return true
		}
	}
	return false
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
