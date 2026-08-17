package api

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"denova/internal/book"
	"denova/internal/book/lore"
	workspacechange "denova/internal/workspace/change"
	"denova/internal/workspace/documentreview"
)

func TestProjectBookAPIKeepsBackgroundBookScoped(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	foreground := application.Workspace()
	background := filepath.Join(t.TempDir(), "background-book")
	if err := book.NewState(background).InitWorkspace(); err != nil {
		t.Fatal(err)
	}
	const chapterPath = "chapters/ch01.md"
	const chapterContent = "# 第一章\n\n跨项目正文。\n"
	if err := book.NewService(background).WriteFile(chapterPath, chapterContent); err != nil {
		t.Fatal(err)
	}
	record, err := application.AgentChat().AddProject(background)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + url.PathEscape(record.ID) + "/book"

	snapshotResponse := performJSONRequest(t, server, http.MethodGet, base, nil)
	if snapshotResponse.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snapshotResponse.Code, snapshotResponse.Body.String())
	}
	var snapshot struct {
		ProjectID string                `json:"project_id"`
		Workspace string                `json:"workspace"`
		Tree      []*book.FileNode      `json:"tree"`
		Summary   book.WorkspaceSummary `json:"summary"`
	}
	decodeResponse(t, snapshotResponse.Body.Bytes(), &snapshot)
	if snapshot.ProjectID != record.ID || snapshot.Workspace != record.WorkspacePath {
		t.Fatalf("snapshot resolved wrong project: %#v", snapshot)
	}
	if len(snapshot.Tree) == 0 || len(snapshot.Summary.Chapters) != 1 || snapshot.Summary.Chapters[0].Path != chapterPath {
		t.Fatalf("snapshot omitted background Book content: %#v", snapshot)
	}

	for _, projection := range []string{"tree", "summary"} {
		response := performJSONRequest(t, server, http.MethodGet, base+"/"+projection, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", projection, response.Code, response.Body.String())
		}
	}

	confirmed := performJSONRequest(t, server, http.MethodPatch, base+"/chapter-status", map[string]any{
		"path": chapterPath, "confirmed": true,
	})
	if confirmed.Code != http.StatusOK {
		t.Fatalf("chapter status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	_, summary, err := application.ProjectBook().Summary(t.Context(), record.ID)
	if err != nil || len(summary.Chapters) != 1 || !summary.Chapters[0].Confirmed {
		t.Fatalf("chapter confirmation was not scoped to background Book: summary=%#v err=%v", summary, err)
	}

	createdResponse := performJSONRequest(t, server, http.MethodPost, base+"/lore/items", map[string]any{
		"enabled": true, "type": "character", "name": "林川", "importance": "major",
		"tags": []string{"主角"}, "brief_description": "调查员", "keywords": []string{"林川"},
		"load_mode": lore.LoadModeResident, "content": "旧正文",
	})
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create lore status=%d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var createdEnvelope struct {
		ProjectID string    `json:"project_id"`
		Item      lore.Item `json:"item"`
	}
	decodeResponse(t, createdResponse.Body.Bytes(), &createdEnvelope)
	if createdEnvelope.ProjectID != record.ID {
		t.Fatalf("create Lore response lost project scope: %#v", createdEnvelope)
	}
	created := createdEnvelope.Item
	updatedResponse := performJSONRequest(t, server, http.MethodPut, base+"/lore/items/"+url.PathEscape(created.ID), map[string]any{
		"id": created.ID, "enabled": created.Enabled, "type": created.Type, "type_source": created.TypeSource,
		"name": "林川（成年）", "importance": created.Importance, "tags": created.Tags,
		"brief_description": created.BriefDescription, "keywords": created.Keywords,
		"load_mode": created.LoadMode, "content": "新正文", "base_revision": created.UpdatedAt,
	})
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("update lore status=%d body=%s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updatedEnvelope struct {
		ProjectID string    `json:"project_id"`
		Item      lore.Item `json:"item"`
	}
	decodeResponse(t, updatedResponse.Body.Bytes(), &updatedEnvelope)
	if updatedEnvelope.ProjectID != record.ID {
		t.Fatalf("update Lore response lost project scope: %#v", updatedEnvelope)
	}
	updated := updatedEnvelope.Item
	if updated.Name != "林川（成年）" || updated.Content != "新正文" {
		t.Fatalf("unexpected updated lore item: %#v", updated)
	}

	start := len("# 第一章\n\n")
	quote := "跨项目正文。"
	commentResponse := performJSONRequest(t, server, http.MethodPost, base+"/document-comments", map[string]any{
		"target": map[string]any{"kind": documentreview.TargetKindWorkspaceFile, "id": chapterPath},
		"body":   "补充人物动机",
		"anchor": map[string]any{
			"kind": documentreview.AnchorKindTextRange, "encoding": documentreview.AnchorEncodingUTF8,
			"revision": workspacechange.Revision([]byte(chapterContent)), "start": start, "end": start + len(quote),
			"quote": quote, "display_quote": quote, "editor_from": 1, "editor_to": 2,
		},
	})
	if commentResponse.Code != http.StatusCreated {
		t.Fatalf("create comment status=%d body=%s", commentResponse.Code, commentResponse.Body.String())
	}
	var commentBody struct {
		ProjectID string                 `json:"project_id"`
		Workspace string                 `json:"workspace"`
		Thread    documentreview.Thread  `json:"review_thread"`
		Comment   documentreview.Comment `json:"comment"`
	}
	decodeResponse(t, commentResponse.Body.Bytes(), &commentBody)
	if commentBody.ProjectID != record.ID || commentBody.Workspace != record.WorkspacePath || len(commentBody.Thread.Comments) != 1 {
		t.Fatalf("comment resolved wrong project: %#v", commentBody)
	}
	listedComments := performJSONRequest(t, server, http.MethodGet, base+"/document-review", nil)
	if listedComments.Code != http.StatusOK {
		t.Fatalf("list comments status=%d body=%s", listedComments.Code, listedComments.Body.String())
	}

	deletedLore := performJSONRequest(t, server, http.MethodDelete, base+"/lore/items/"+url.PathEscape(created.ID), nil)
	if deletedLore.Code != http.StatusOK {
		t.Fatalf("delete lore status=%d body=%s", deletedLore.Code, deletedLore.Body.String())
	}
	listedLore := performJSONRequest(t, server, http.MethodGet, base+"/lore/items", nil)
	var loreList struct {
		Items []lore.Item `json:"items"`
	}
	decodeResponse(t, listedLore.Body.Bytes(), &loreList)
	if listedLore.Code != http.StatusOK || len(loreList.Items) != 0 {
		t.Fatalf("deleted Lore item remained: status=%d body=%s", listedLore.Code, listedLore.Body.String())
	}
	if application.Workspace() != foreground {
		t.Fatalf("Project Book API switched foreground workspace: got=%q want=%q", application.Workspace(), foreground)
	}
}
