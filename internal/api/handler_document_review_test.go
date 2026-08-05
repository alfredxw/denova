package api

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"denova/internal/book/lore"
	workspacechange "denova/internal/workspace/change"
	"denova/internal/workspace/documentreview"
)

func TestDocumentReviewCommentLifecycleAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book"
	path := "chapters/review.md"
	content := "第一段。\n\n第二段需要审阅。\n"
	if err := application.BookService().WriteFile(path, content); err != nil {
		t.Fatal(err)
	}
	start := len("第一段。\n\n")
	quote := "第二段需要审阅。"
	anchor := map[string]any{
		"kind": documentreview.AnchorKindTextBlock, "encoding": documentreview.AnchorEncodingUTF8,
		"revision": workspacechange.Revision([]byte(content)), "start": start, "end": start + len(quote),
		"quote": quote, "suffix": "\n", "display_quote": quote, "editor_from": 5, "editor_to": 13,
	}

	created := performJSONRequest(t, server, http.MethodPost, base+"/document-comments", map[string]any{
		"target": map[string]any{"kind": documentreview.TargetKindWorkspaceFile, "id": path},
		"body":   "补充人物动机", "anchor": anchor,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var createBody struct {
		ProjectID    string                 `json:"project_id"`
		Workspace    string                 `json:"workspace"`
		ReviewThread documentreview.Thread  `json:"review_thread"`
		Comment      documentreview.Comment `json:"comment"`
	}
	decodeResponse(t, created.Body.Bytes(), &createBody)
	if createBody.ProjectID != projectID || createBody.Workspace != workspace || createBody.ReviewThread.ID == "" || createBody.Comment.ThreadID != createBody.ReviewThread.ID {
		t.Fatalf("unexpected create response: %#v", createBody)
	}

	listed := performJSONRequest(t, server, http.MethodGet, base+"/document-review", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	var listBody struct {
		ReviewThread documentreview.Thread `json:"review_thread"`
	}
	decodeResponse(t, listed.Body.Bytes(), &listBody)
	if len(listBody.ReviewThread.Comments) != 1 || listBody.ReviewThread.Comments[0].Body != "补充人物动机" {
		t.Fatalf("unexpected review thread: %#v", listBody.ReviewThread)
	}

	updated := performJSONRequest(t, server, http.MethodPatch, base+"/document-comments/"+createBody.Comment.ID, map[string]any{"body": "补充更明确的人物动机"})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	deleted := performJSONRequest(t, server, http.MethodDelete, base+"/document-comments/"+createBody.Comment.ID, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	empty := performJSONRequest(t, server, http.MethodGet, base+"/document-review", nil)
	decodeResponse(t, empty.Body.Bytes(), &listBody)
	if listBody.ReviewThread.ID != "" || len(listBody.ReviewThread.Comments) != 0 {
		t.Fatalf("deleted comment remained pending: %#v", listBody.ReviewThread)
	}

	forged := performJSONRequest(t, server, http.MethodPost, base+"/document-comments", map[string]any{
		"target": map[string]any{"kind": documentreview.TargetKindWorkspaceFile, "id": path},
		"body":   "伪造评论", "anchor": map[string]any{
			"kind": documentreview.AnchorKindTextRange, "encoding": documentreview.AnchorEncodingUTF8,
			"revision": workspacechange.Revision([]byte(content)), "start": start, "end": start + len(quote),
			"quote": "并不存在的原文", "display_quote": quote, "editor_from": 5, "editor_to": 13,
		},
	})
	if forged.Code != http.StatusConflict {
		t.Fatalf("forged anchor status=%d body=%s workspace=%s", forged.Code, forged.Body.String(), filepath.Base(workspace))
	}
}

func TestLoreReviewCommentLifecycleAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book"
	item, err := application.ProjectBook().CreateLoreItem(projectID, lore.ItemInput{
		ID: "gatekeeper", Type: "character", Name: "守门人", Content: "Aldren guards the northern gate.",
	})
	if err != nil {
		t.Fatal(err)
	}
	quote := "guards the northern gate"
	start := len("Aldren ")
	created := performJSONRequest(t, server, http.MethodPost, base+"/document-comments", map[string]any{
		"target": map[string]any{
			"kind":  documentreview.TargetKindLoreItem,
			"id":    item.ID,
			"field": documentreview.TargetFieldLoreContent,
		},
		"body": "Clarify why Aldren is guarding it.",
		"anchor": map[string]any{
			"kind": documentreview.AnchorKindTextRange, "encoding": documentreview.AnchorEncodingUTF8,
			"revision": item.UpdatedAt, "start": start, "end": start + len(quote),
			"quote": quote, "display_quote": quote, "editor_from": 2, "editor_to": 8,
		},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create lore comment status=%d body=%s", created.Code, created.Body.String())
	}
	var createBody struct {
		Comment documentreview.Comment `json:"comment"`
	}
	decodeResponse(t, created.Body.Bytes(), &createBody)
	if createBody.Comment.Target.Kind != documentreview.TargetKindLoreItem || createBody.Comment.Target.ID != item.ID {
		t.Fatalf("unexpected lore review target: %#v", createBody.Comment.Target)
	}

	listed := performJSONRequest(t, server, http.MethodGet, base+"/document-review", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	var listBody struct {
		ReviewThread documentreview.Thread `json:"review_thread"`
	}
	decodeResponse(t, listed.Body.Bytes(), &listBody)
	if len(listBody.ReviewThread.Comments) != 1 || listBody.ReviewThread.Comments[0].Target.ID != item.ID {
		t.Fatalf("unexpected lore review thread: %#v", listBody.ReviewThread)
	}
}
