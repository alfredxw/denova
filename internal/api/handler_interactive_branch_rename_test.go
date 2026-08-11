package api

import (
	"net/http"
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveBranchRenameAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "末日开端",
		"origin":          "主角醒来发现世界已末日",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)

	if _, err := application.AppendInteractiveTurn(created.ID, "", "我推开酒馆的门", "门后传来低沉的风声。"); err != nil {
		t.Fatal(err)
	}
	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot struct {
		Turns []map[string]any `json:"turns"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if len(snapshot.Turns) != 1 {
		t.Fatalf("expected one turn: %#v", snapshot.Turns)
	}
	parentEventID, _ := snapshot.Turns[0]["id"].(string)
	if parentEventID == "" {
		t.Fatalf("turn id missing: %#v", snapshot.Turns[0])
	}

	branchResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/branches", map[string]string{
		"parent_event_id": parentEventID,
		"title":           "换条路走",
	})
	if branchResp.Code != http.StatusOK {
		t.Fatalf("create branch status = %d body=%s", branchResp.Code, branchResp.Body.String())
	}
	var createdBranch interactive.BranchSummary
	decodeResponse(t, branchResp.Body.Bytes(), &createdBranch)
	if createdBranch.ID == "" || createdBranch.Title != "换条路走" {
		t.Fatalf("created branch mismatch: %#v", createdBranch)
	}

	renameResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID+"/branches/"+createdBranch.ID, map[string]string{
		"title": "密林小径",
	})
	if renameResp.Code != http.StatusOK {
		t.Fatalf("rename branch status = %d body=%s", renameResp.Code, renameResp.Body.String())
	}
	var renamed interactive.BranchSummary
	decodeResponse(t, renameResp.Body.Bytes(), &renamed)
	if renamed.ID != createdBranch.ID || renamed.Title != "密林小径" {
		t.Fatalf("renamed branch mismatch: %#v", renamed)
	}

	branchesResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/branches", nil)
	if branchesResp.Code != http.StatusOK {
		t.Fatalf("list branches status = %d body=%s", branchesResp.Code, branchesResp.Body.String())
	}
	var listed struct {
		Branches []interactive.BranchSummary `json:"branches"`
	}
	decodeResponse(t, branchesResp.Body.Bytes(), &listed)
	if !containsBranchTitle(listed.Branches, createdBranch.ID, "密林小径") {
		t.Fatalf("branch list did not return the new title: %#v", listed.Branches)
	}

	graphSnapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot?branch="+createdBranch.ID, nil)
	if graphSnapshotResp.Code != http.StatusOK {
		t.Fatalf("graph snapshot status = %d body=%s", graphSnapshotResp.Code, graphSnapshotResp.Body.String())
	}
	var graphSnapshot struct {
		Graph struct {
			Branches []interactive.BranchSummary `json:"branches"`
		} `json:"graph"`
	}
	decodeResponse(t, graphSnapshotResp.Body.Bytes(), &graphSnapshot)
	if !containsBranchTitle(graphSnapshot.Graph.Branches, createdBranch.ID, "密林小径") {
		t.Fatalf("snapshot graph did not return the new title: %#v", graphSnapshot.Graph.Branches)
	}

	emptyTitleResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID+"/branches/"+createdBranch.ID, map[string]string{
		"title": "   ",
	})
	if emptyTitleResp.Code != http.StatusBadRequest {
		t.Fatalf("empty title should be rejected, status = %d body=%s", emptyTitleResp.Code, emptyTitleResp.Body.String())
	}
}

func containsBranchTitle(branches []interactive.BranchSummary, branchID, title string) bool {
	for _, branch := range branches {
		if branch.ID == branchID && branch.Title == title {
			return true
		}
	}
	return false
}
