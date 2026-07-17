package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/internal/workspacechange"
)

func TestWorkspaceChangeReviewCommentUndoRedoAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "第一段\n第二段"); err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	service, err := application.WorkspaceChangeService()
	if err != nil {
		t.Fatalf("change service: %v", err)
	}
	_, baseRevision, err := service.ReadFile("chapters/ch01.md")
	if err != nil {
		t.Fatalf("read chapter revision: %v", err)
	}
	change, err := service.ApplyEdits(context.Background(), workspacechange.ApplyEditsRequest{
		Path:         "chapters/ch01.md",
		BaseRevision: baseRevision,
		Edits:        []workspacechange.TextEdit{{ID: "edit-1", OldString: "第二段", NewString: "Agent 第二段"}},
		Metadata: workspacechange.ChangeMetadata{
			Origin:        workspacechange.OriginAgent,
			ChangeGroupID: "run-1",
			RunID:         "run-1",
			SessionID:     "default",
		},
	})
	if err != nil {
		t.Fatalf("apply edit: %v", err)
	}
	workspace := application.Workspace()

	listResp := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-groups?status=pending", workspace, nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var list struct {
		Workspace string                               `json:"workspace"`
		Groups    []workspacechange.ChangeGroupSummary `json:"groups"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &list)
	if list.Workspace != workspace || len(list.Groups) != 1 || list.Groups[0].ID != "run-1" {
		t.Fatalf("unexpected groups: %#v", list.Groups)
	}

	detailResp := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-groups/run-1", workspace, nil)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		Workspace string                      `json:"workspace"`
		Group     workspacechange.ChangeGroup `json:"group"`
	}
	decodeResponse(t, detailResp.Body.Bytes(), &detail)
	if detail.Workspace != workspace || len(detail.Group.ChangeSets) != 1 || detail.Group.ChangeSets[0].BeforeContent == "" || detail.Group.ChangeSets[0].AfterContent == "" {
		t.Fatalf("detail should hydrate diff content: %#v", detail.Group)
	}

	commentResp := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-comments", workspace, map[string]any{
		"group_id":      "run-1",
		"change_set_id": change.ID,
		"edit_id":       "edit-1",
		"body":          "这里的人称需要确认",
	})
	if commentResp.Code != http.StatusCreated {
		t.Fatalf("comment status=%d body=%s", commentResp.Code, commentResp.Body.String())
	}
	var commentBody struct {
		Workspace string                  `json:"workspace"`
		Comment   workspacechange.Comment `json:"comment"`
	}
	decodeResponse(t, commentResp.Body.Bytes(), &commentBody)
	if commentBody.Workspace != workspace {
		t.Fatalf("comment workspace=%q want=%q", commentBody.Workspace, workspace)
	}

	threadResp := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-review-threads/run-1", workspace, nil)
	if threadResp.Code != http.StatusOK {
		t.Fatalf("review thread status=%d body=%s", threadResp.Code, threadResp.Body.String())
	}
	var threadBody struct {
		Workspace    string                       `json:"workspace"`
		ReviewThread workspacechange.ReviewThread `json:"review_thread"`
	}
	decodeResponse(t, threadResp.Body.Bytes(), &threadBody)
	if threadBody.Workspace != workspace || threadBody.ReviewThread.ID != "run-1" || len(threadBody.ReviewThread.Files) != 1 || len(threadBody.ReviewThread.Comments) != 1 {
		t.Fatalf("unexpected review thread response: %#v", threadBody)
	}

	feedbackResp := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "请处理审阅意见",
		"review_feedback": map[string]any{
			"review_thread_id": "run-1",
			"comment_ids":      []string{commentBody.Comment.ID},
			"comments":         []map[string]string{{"body": "FORGED CLIENT COMMENT"}},
		},
	})
	if feedbackResp.Code != http.StatusOK {
		t.Fatalf("review feedback analysis status=%d body=%s", feedbackResp.Code, feedbackResp.Body.String())
	}
	if body := feedbackResp.Body.String(); !strings.Contains(body, "这里的人称需要确认") || !strings.Contains(body, "durable change ledger") || strings.Contains(body, "FORGED CLIENT COMMENT") {
		t.Fatalf("review feedback was not resolved exclusively from the ledger: %s", body)
	}

	forgedFeedbackResp := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "请处理审阅意见",
		"review_feedback": map[string]any{
			"review_thread_id": "run-1",
			"comment_ids":      []string{"forged-comment"},
		},
	})
	if forgedFeedbackResp.Code != http.StatusNotFound || !strings.Contains(forgedFeedbackResp.Body.String(), `"code":"not_found"`) {
		t.Fatalf("forged review feedback status=%d body=%s", forgedFeedbackResp.Code, forgedFeedbackResp.Body.String())
	}

	updateCommentResp := performWorkspaceChangeRequest(t, server, http.MethodPatch, "/api/workspace/change-comments/"+commentBody.Comment.ID, workspace, map[string]any{
		"body": "这里的人称已经确认",
	})
	if updateCommentResp.Code != http.StatusOK {
		t.Fatalf("update comment status=%d body=%s", updateCommentResp.Code, updateCommentResp.Body.String())
	}
	resolveCommentResp := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-comments/"+commentBody.Comment.ID+"/resolve", workspace, map[string]any{
		"resolved": true,
	})
	if resolveCommentResp.Code != http.StatusOK {
		t.Fatalf("resolve comment status=%d body=%s", resolveCommentResp.Code, resolveCommentResp.Body.String())
	}
	deleteCommentResp := performWorkspaceChangeRequest(t, server, http.MethodDelete, "/api/workspace/change-comments/"+commentBody.Comment.ID, workspace, nil)
	if deleteCommentResp.Code != http.StatusOK {
		t.Fatalf("delete comment status=%d body=%s", deleteCommentResp.Code, deleteCommentResp.Body.String())
	}

	reviewResp := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-groups/run-1/review", workspace, map[string]any{
		"decision":      "accept",
		"change_set_id": change.ID,
		"edit_ids":      []string{"edit-1"},
	})
	if reviewResp.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", reviewResp.Code, reviewResp.Body.String())
	}

	undoResp := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-groups/run-1/undo", workspace, nil)
	if undoResp.Code != http.StatusOK {
		t.Fatalf("undo status=%d body=%s", undoResp.Code, undoResp.Body.String())
	}
	content, err := application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || content != "第一段\n第二段" {
		t.Fatalf("undo content=%q err=%v", content, err)
	}

	redoResp := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-groups/run-1/redo", workspace, nil)
	if redoResp.Code != http.StatusOK {
		t.Fatalf("redo status=%d body=%s", redoResp.Code, redoResp.Body.String())
	}
	content, err = application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || content != "第一段\nAgent 第二段" {
		t.Fatalf("redo content=%q err=%v", content, err)
	}
}

func TestWorkspaceChangeReviewResponseUsesOperationScopedPaths(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	for path, content := range map[string]string{
		"chapters/ch01.md": "chapter draft",
		"setting/world.md": "world draft",
	} {
		if err := application.BookService().Create(path, "file", content); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
	}
	service, err := application.WorkspaceChangeService()
	if err != nil {
		t.Fatal(err)
	}
	metadata := workspacechange.ChangeMetadata{Origin: workspacechange.OriginAgent, ChangeGroupID: "selective-api-review"}
	chapter, err := service.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: "chapters/ch01.md", Content: "chapter agent", BaseRevision: workspacechange.Revision([]byte("chapter draft")), Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: "setting/world.md", Content: "world agent", BaseRevision: workspacechange.Revision([]byte("world draft")), Metadata: metadata,
	}); err != nil {
		t.Fatal(err)
	}

	response := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-groups/selective-api-review/review", application.Workspace(), map[string]any{
		"decision": "reject", "change_set_id": chapter.ID,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		AffectedPaths []string `json:"affected_paths"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if len(body.AffectedPaths) != 1 || body.AffectedPaths[0] != "chapters/ch01.md" {
		t.Fatalf("affected paths=%#v", body.AffectedPaths)
	}
	if content, err := application.BookService().ReadFile("setting/world.md"); err != nil || content != "world agent" {
		t.Fatalf("unselected file content=%q err=%v", content, err)
	}
}

func TestWorkspaceChangeAPIRequiresCanonicalWorkspaceLease(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()

	missing := performJSONRequest(t, server, http.MethodGet, "/api/workspace/change-groups", nil)
	assertWorkspaceChangedResponse(t, missing, "", workspace)
	missingMutation := performJSONRequest(t, server, http.MethodPost, "/api/workspace/change-groups/unknown/review", map[string]any{
		"decision": "accept",
	})
	assertWorkspaceChangedResponse(t, missingMutation, "", workspace)

	staleWorkspace := filepath.Join(workspace, "stale")
	stale := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-groups", staleWorkspace, nil)
	assertWorkspaceChangedResponse(t, stale, staleWorkspace, workspace)

	valid := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-groups", workspace, nil)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid lease status=%d body=%s", valid.Code, valid.Body.String())
	}
	var body struct {
		Workspace string `json:"workspace"`
	}
	decodeResponse(t, valid.Body.Bytes(), &body)
	if body.Workspace != workspace {
		t.Fatalf("response workspace=%q want=%q", body.Workspace, workspace)
	}
}

func TestWorkspaceSwitchCanonicalizesSymlinkIdentity(t *testing.T) {
	application := newTestApplication(t)
	workspace := application.Workspace()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatal(err)
	}
	got, err := application.SwitchWorkspace(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if got != workspace || application.Workspace() != workspace {
		t.Fatalf("workspace alias was not canonicalized: result=%q current=%q want=%q", got, application.Workspace(), workspace)
	}
	service, err := application.WorkspaceChangeService()
	if err != nil {
		t.Fatal(err)
	}
	if service.Workspace() != workspace {
		t.Fatalf("change service identity=%q want=%q", service.Workspace(), workspace)
	}
}

func TestWorkspaceChangeAPIRejectsLeaseAfterWorkspaceSwitch(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	previousWorkspace := application.Workspace()
	nextWorkspace := t.TempDir()
	if _, err := application.SwitchWorkspace(context.Background(), nextWorkspace); err != nil {
		t.Fatalf("switch workspace: %v", err)
	}
	currentWorkspace := application.Workspace()

	response := performWorkspaceChangeRequest(t, server, http.MethodGet, "/api/workspace/change-groups", previousWorkspace, nil)
	assertWorkspaceChangedResponse(t, response, previousWorkspace, currentWorkspace)

	mutationResponse := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/workspace/change-groups/old-group/review", previousWorkspace, map[string]any{
		"decision": "accept",
	})
	assertWorkspaceChangedResponse(t, mutationResponse, previousWorkspace, currentWorkspace)
}

func TestWorkspaceChangeAPIKeepsStructuredConflict(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "base"); err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	writeResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/file", map[string]any{
		"path":          "chapters/ch01.md",
		"content":       "stale write",
		"base_revision": "sha256:stale",
		"workspace":     application.Workspace(),
	})
	if writeResp.Code != http.StatusConflict {
		t.Fatalf("write status=%d body=%s", writeResp.Code, writeResp.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	decodeResponse(t, writeResp.Body.Bytes(), &body)
	if body.Code != workspacechange.ErrorCodeRevisionConflict || body.Details["actual_revision"] == "" {
		t.Fatalf("structured conflict missing: %#v", body)
	}
}

func performWorkspaceChangeRequest(t *testing.T, server *Server, method, path, workspace string, body any) *ut.ResponseRecorder {
	t.Helper()
	var requestBody *ut.Body
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = &ut.Body{Body: bytes.NewReader(data), Len: len(data)}
	}
	return ut.PerformRequest(
		server.engine.Engine,
		method,
		path,
		requestBody,
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Denova-Workspace", Value: url.PathEscape(workspace)},
	)
}

func assertWorkspaceChangedResponse(t *testing.T, response *ut.ResponseRecorder, expectedWorkspace, actualWorkspace string) {
	t.Helper()
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code    string            `json:"code"`
		Details map[string]string `json:"details"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if body.Code != "workspace_changed" || body.Details["expected_workspace"] != expectedWorkspace || body.Details["actual_workspace"] != actualWorkspace {
		t.Fatalf("unexpected workspace conflict: %#v", body)
	}
}
