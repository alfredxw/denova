package api

import (
	"net/http"
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveStateRevisionLifecycleAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "State revision story",
		"story_teller_id": "classic",
	})
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create story status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var story interactive.StorySummary
	decodeResponse(t, createResponse.Body.Bytes(), &story)

	turn, err := application.AppendInteractiveTurn(story.ID, "main", "Enter the observatory", "Rain strikes the glass dome.")
	if err != nil {
		t.Fatal(err)
	}
	createRevisionResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+story.ID+"/state-revisions", map[string]any{
		"branch_id":        "main",
		"expected_head_id": turn.ID,
		"base_turn_id":     turn.ID,
		"source":           "manual_state_editor",
		"ops": []map[string]any{{
			"op":     "set",
			"path":   "scene.weather",
			"value":  "storm",
			"reason": "Correct the settled scene state",
		}},
	})
	if createRevisionResponse.Code != http.StatusOK {
		t.Fatalf("create state revision status=%d body=%s", createRevisionResponse.Code, createRevisionResponse.Body.String())
	}
	var created interactive.StateRevisionEvent
	decodeResponse(t, createRevisionResponse.Body.Bytes(), &created)
	if created.ID == "" || created.BranchID != "main" || created.BaseTurnID != turn.ID || created.Source != "manual_state_editor" || created.Action != interactive.StateRevisionActionApply || created.Ts == "" {
		t.Fatalf("state revision audit metadata mismatch: %#v", created)
	}

	snapshot := readInteractiveSnapshotResponse(t, server, story.ID, "main")
	if snapshot.HeadID != created.ID {
		t.Fatalf("snapshot head after state revision=%q want=%q", snapshot.HeadID, created.ID)
	}
	if weather := nestedStateString(snapshot.State, "scene", "weather"); weather != "storm" {
		t.Fatalf("state revision was not replayed: weather=%q snapshot=%#v", weather, snapshot)
	}
	if len(snapshot.StateRevisions) != 1 || snapshot.StateRevisions[0].ID != created.ID {
		t.Fatalf("snapshot state revision history mismatch: %#v", snapshot.StateRevisions)
	}

	undoResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+story.ID+"/state-revisions/"+created.ID+"/undo", map[string]string{
		"branch_id":        "main",
		"expected_head_id": created.ID,
		"source":           "manual_state_editor",
	})
	if undoResponse.Code != http.StatusOK {
		t.Fatalf("undo state revision status=%d body=%s", undoResponse.Code, undoResponse.Body.String())
	}
	var undone interactive.StateRevisionEvent
	decodeResponse(t, undoResponse.Body.Bytes(), &undone)
	if undone.Action != interactive.StateRevisionActionUndo || undone.SourceRevisionID != created.ID || undone.BaseTurnID != turn.ID {
		t.Fatalf("undo audit metadata mismatch: %#v", undone)
	}
	snapshot = readInteractiveSnapshotResponse(t, server, story.ID, "main")
	if snapshot.HeadID != undone.ID {
		t.Fatalf("snapshot head after undo=%q want=%q", snapshot.HeadID, undone.ID)
	}
	if weather := nestedStateString(snapshot.State, "scene", "weather"); weather != "" {
		t.Fatalf("undo must restore the pre-revision state: weather=%q", weather)
	}

	restoreResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+story.ID+"/state-revisions/"+created.ID+"/restore", map[string]string{
		"branch_id":        "main",
		"expected_head_id": undone.ID,
		"source":           "manual_state_editor",
	})
	if restoreResponse.Code != http.StatusOK {
		t.Fatalf("restore state revision status=%d body=%s", restoreResponse.Code, restoreResponse.Body.String())
	}
	var restored interactive.StateRevisionEvent
	decodeResponse(t, restoreResponse.Body.Bytes(), &restored)
	if restored.Action != interactive.StateRevisionActionRestore || restored.SourceRevisionID != created.ID {
		t.Fatalf("restore audit metadata mismatch: %#v", restored)
	}
	snapshot = readInteractiveSnapshotResponse(t, server, story.ID, "main")
	if snapshot.HeadID != restored.ID {
		t.Fatalf("snapshot head after restore=%q want=%q", snapshot.HeadID, restored.ID)
	}
	if weather := nestedStateString(snapshot.State, "scene", "weather"); weather != "storm" {
		t.Fatalf("restore must replay the revision: weather=%q", weather)
	}

	branchResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+story.ID+"/branches", map[string]string{
		"parent_event_id": turn.ID,
		"title":           "Dry branch",
	})
	if branchResponse.Code != http.StatusOK {
		t.Fatalf("create branch status=%d body=%s", branchResponse.Code, branchResponse.Body.String())
	}
	var branch interactive.BranchSummary
	decodeResponse(t, branchResponse.Body.Bytes(), &branch)
	branchSnapshot := readInteractiveSnapshotResponse(t, server, story.ID, branch.ID)
	if weather := nestedStateString(branchSnapshot.State, "scene", "weather"); weather != "" || len(branchSnapshot.StateRevisions) != 0 {
		t.Fatalf("state revision leaked into a sibling branch: weather=%q revisions=%#v", weather, branchSnapshot.StateRevisions)
	}

	staleResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+story.ID+"/state-revisions", map[string]any{
		"branch_id":        "main",
		"expected_head_id": created.ID,
		"base_turn_id":     turn.ID,
		"source":           "manual_state_editor",
		"ops":              []map[string]any{{"op": "set", "path": "scene.weather", "value": "clear"}},
	})
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale state revision status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
}

func readInteractiveSnapshotResponse(t *testing.T, server *Server, storyID, branchID string) interactive.Snapshot {
	t.Helper()
	response := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+storyID+"/snapshot?branch="+branchID, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", response.Code, response.Body.String())
	}
	var snapshot interactive.Snapshot
	decodeResponse(t, response.Body.Bytes(), &snapshot)
	return snapshot
}

func nestedStateString(state map[string]any, group, field string) string {
	groupState, _ := state[group].(map[string]any)
	value, _ := groupState[field].(string)
	return value
}
