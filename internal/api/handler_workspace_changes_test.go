package api

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	agentreview "denova/internal/agents/review"
	workspacechange "denova/internal/workspace/change"
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

	listResp := performProjectChangeRequest(t, server, http.MethodGet, application.ProjectID(), "/groups?status=pending", nil)
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

	detailResp := performProjectChangeRequest(t, server, http.MethodGet, application.ProjectID(), "/groups/run-1", nil)
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

	commentResp := performProjectChangeRequest(t, server, http.MethodPost, application.ProjectID(), "/comments", map[string]any{
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

	threadResp := performProjectChangeRequest(t, server, http.MethodGet, application.ProjectID(), "/review-threads/run-1", nil)
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
		"review_feedback": []map[string]any{{
			"review_thread_id": "run-1",
			"comment_ids":      []string{commentBody.Comment.ID},
			"comments":         []map[string]string{{"body": "FORGED CLIENT COMMENT"}},
		}},
	})
	if feedbackResp.Code != http.StatusOK {
		t.Fatalf("review feedback analysis status=%d body=%s", feedbackResp.Code, feedbackResp.Body.String())
	}
	var analysis agentchat.ContextAnalysis
	decodeResponse(t, feedbackResp.Body.Bytes(), &analysis)
	var trustedFeedback agentreview.Contexts
	for _, part := range analysis.ContextParts {
		if part.Source != "workspace.review.feedback" {
			continue
		}
		const fence = "```json\n"
		start := strings.Index(part.Content, fence)
		end := strings.LastIndex(part.Content, "\n```")
		if start < 0 || end <= start+len(fence) {
			t.Fatalf("review feedback context is not a JSON block: %q", part.Content)
		}
		if err := json.Unmarshal([]byte(part.Content[start+len(fence):end]), &trustedFeedback); err != nil {
			t.Fatalf("decode trusted review feedback: %v content=%q", err, part.Content)
		}
		break
	}
	if len(trustedFeedback) != 1 || trustedFeedback[0].Source != agentreview.SourceWorkspaceChange || len(trustedFeedback[0].Comments) != 1 || trustedFeedback[0].Comments[0].Body != "这里的人称需要确认" {
		t.Fatalf("review feedback was not resolved exclusively from the ledger: %#v", trustedFeedback)
	}
	if strings.Contains(feedbackResp.Body.String(), "FORGED CLIENT COMMENT") {
		t.Fatalf("forged client review feedback reached context analysis: %s", feedbackResp.Body.String())
	}

	forgedFeedbackResp := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "请处理审阅意见",
		"review_feedback": []map[string]any{{
			"review_thread_id": "run-1",
			"comment_ids":      []string{"forged-comment"},
		}},
	})
	if forgedFeedbackResp.Code != http.StatusNotFound || !strings.Contains(forgedFeedbackResp.Body.String(), `"code":"not_found"`) {
		t.Fatalf("forged review feedback status=%d body=%s", forgedFeedbackResp.Code, forgedFeedbackResp.Body.String())
	}

	updateCommentResp := performProjectChangeRequest(t, server, http.MethodPatch, application.ProjectID(), "/comments/"+commentBody.Comment.ID, map[string]any{
		"body": "这里的人称已经确认",
	})
	if updateCommentResp.Code != http.StatusOK {
		t.Fatalf("update comment status=%d body=%s", updateCommentResp.Code, updateCommentResp.Body.String())
	}
	deleteCommentResp := performProjectChangeRequest(t, server, http.MethodDelete, application.ProjectID(), "/comments/"+commentBody.Comment.ID, nil)
	if deleteCommentResp.Code != http.StatusOK {
		t.Fatalf("delete comment status=%d body=%s", deleteCommentResp.Code, deleteCommentResp.Body.String())
	}

	reviewResp := performProjectChangeRequest(t, server, http.MethodPost, application.ProjectID(), "/groups/run-1/review", map[string]any{
		"decision":      "accept",
		"change_set_id": change.ID,
		"edit_ids":      []string{"edit-1"},
	})
	if reviewResp.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", reviewResp.Code, reviewResp.Body.String())
	}

	undoResp := performProjectChangeRequest(t, server, http.MethodPost, application.ProjectID(), "/groups/run-1/undo", nil)
	if undoResp.Code != http.StatusOK {
		t.Fatalf("undo status=%d body=%s", undoResp.Code, undoResp.Body.String())
	}
	content, err := application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || content != "第一段\n第二段" {
		t.Fatalf("undo content=%q err=%v", content, err)
	}

	redoResp := performProjectChangeRequest(t, server, http.MethodPost, application.ProjectID(), "/groups/run-1/redo", nil)
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

	response := performProjectChangeRequest(t, server, http.MethodPost, application.ProjectID(), "/groups/selective-api-review/review", map[string]any{
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

func TestWorkspaceSwitchCanonicalizesSymlinkIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires optional Windows symlink privileges")
	}
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

func TestProjectChangeAPIKeepsBackgroundProjectScoped(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	workspace := application.Workspace()
	if err := application.BookService().Create("chapters/ch01.md", "file", "base"); err != nil {
		t.Fatal(err)
	}
	service, err := application.WorkspaceChangeService()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: "chapters/ch01.md", Content: "background", BaseRevision: workspacechange.Revision([]byte("base")),
		Metadata: workspacechange.ChangeMetadata{Origin: workspacechange.OriginAgent, ChangeGroupID: "background-change"},
	}); err != nil {
		t.Fatal(err)
	}
	created := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "Foreground Change Book"})
	if created.Code != http.StatusOK || application.ProjectID() == projectID {
		t.Fatalf("create foreground Book status=%d project_id=%q body=%s", created.Code, application.ProjectID(), created.Body.String())
	}

	response := performProjectChangeRequest(t, server, http.MethodGet, projectID, "/groups", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("background Project list status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		ProjectID string                               `json:"project_id"`
		Workspace string                               `json:"workspace"`
		Groups    []workspacechange.ChangeGroupSummary `json:"groups"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if body.ProjectID != projectID || body.Workspace != workspace || len(body.Groups) != 1 || body.Groups[0].ID != "background-change" {
		t.Fatalf("unexpected background Project change response: %#v", body)
	}
	if _, err := application.BookService().ReadFile("chapters/ch01.md"); !os.IsNotExist(err) {
		t.Fatalf("foreground Book must remain isolated, err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "chapters", "ch01.md"))
	if err != nil || string(data) != "background" {
		t.Fatalf("background Project content=%q err=%v", data, err)
	}
}

func TestWorkspaceChangeAPIKeepsStructuredConflict(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "base"); err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	writeResp := performJSONRequest(t, server, http.MethodPut, "/api/projects/"+url.PathEscape(application.ProjectID())+"/files/file", map[string]any{
		"path":          "chapters/ch01.md",
		"content":       "stale write",
		"base_revision": "sha256:stale",
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

func performProjectChangeRequest(t *testing.T, server *Server, method, projectID, suffix string, body any) *ut.ResponseRecorder {
	t.Helper()
	return performJSONRequest(t, server, method, "/api/projects/"+url.PathEscape(projectID)+"/changes"+suffix, body)
}
