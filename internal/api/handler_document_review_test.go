package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"denova/internal/documentreview"
	"denova/internal/workspacechange"
)

func TestDocumentReviewCommentLifecycleAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()
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

	created := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/document-comments", workspace, map[string]any{
		"path": path, "body": "补充人物动机", "anchor": anchor,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var createBody struct {
		Workspace    string                 `json:"workspace"`
		ReviewThread documentreview.Thread  `json:"review_thread"`
		Comment      documentreview.Comment `json:"comment"`
	}
	decodeResponse(t, created.Body.Bytes(), &createBody)
	if createBody.Workspace != workspace || createBody.ReviewThread.ID == "" || createBody.Comment.ThreadID != createBody.ReviewThread.ID {
		t.Fatalf("unexpected create response: %#v", createBody)
	}

	listed := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/document-review", workspace, nil)
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

	updated := performWorkspaceChangeRequest(t, server, http.MethodPatch, "/api/workspace/document-comments/"+createBody.Comment.ID, workspace, map[string]any{"body": "补充更明确的人物动机"})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	deleted := performWorkspaceChangeRequest(t, server, http.MethodDelete, "/api/workspace/document-comments/"+createBody.Comment.ID, workspace, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	empty := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/document-review", workspace, nil)
	decodeResponse(t, empty.Body.Bytes(), &listBody)
	if listBody.ReviewThread.ID != "" || len(listBody.ReviewThread.Comments) != 0 {
		t.Fatalf("deleted comment remained pending: %#v", listBody.ReviewThread)
	}

	forged := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/document-comments", workspace, map[string]any{
		"path": path, "body": "伪造评论", "anchor": map[string]any{
			"kind": documentreview.AnchorKindTextRange, "encoding": documentreview.AnchorEncodingUTF8,
			"revision": workspacechange.Revision([]byte(content)), "start": start, "end": start + len(quote),
			"quote": "并不存在的原文", "display_quote": quote, "editor_from": 5, "editor_to": 13,
		},
	})
	if forged.Code != http.StatusConflict {
		t.Fatalf("forged anchor status=%d body=%s workspace=%s", forged.Code, forged.Body.String(), filepath.Base(workspace))
	}
}
