package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/internal/workspacechange"
)

type recordingWorkspaceChangeService struct {
	workspace      string
	readCalls      int
	readPath       string
	readRevision   string
	readErr        error
	applyCalls     int
	replaceCalls   int
	applyRequest   workspacechange.ApplyEditsRequest
	replaceRequest workspacechange.ReplaceFileRequest
	changeSet      workspacechange.ChangeSet
	err            error
}

func (service *recordingWorkspaceChangeService) Workspace() string { return service.workspace }

func (service *recordingWorkspaceChangeService) ReadFile(path string) (string, string, error) {
	service.readCalls++
	service.readPath = path
	if service.readErr != nil {
		return "", "", service.readErr
	}
	revision := service.readRevision
	if revision == "" {
		revision = "sha256:current"
	}
	return "", revision, nil
}

func (service *recordingWorkspaceChangeService) ApplyEdits(_ context.Context, request workspacechange.ApplyEditsRequest) (workspacechange.ChangeSet, error) {
	service.applyCalls++
	service.applyRequest = request
	return service.changeSet, service.err
}

func (service *recordingWorkspaceChangeService) ReplaceFile(_ context.Context, request workspacechange.ReplaceFileRequest) (workspacechange.ChangeSet, error) {
	service.replaceCalls++
	service.replaceRequest = request
	return service.changeSet, service.err
}

func TestWorkspaceMutationAdapterMapsSingleExactEditAndReturnsReviewReceipt(t *testing.T) {
	service := &recordingWorkspaceChangeService{
		workspace: t.TempDir(), readRevision: "sha256:before",
		changeSet: workspacechange.ChangeSet{
			ID: "change-1", GroupID: "run-1", ReviewThreadID: "review-1",
			Path: "chapters/ch01.md", BaseRevision: "sha256:before", Revision: "sha256:after",
			BeforeContent: strings.Repeat("secret-before", 100), AfterContent: strings.Repeat("secret-after", 100),
			Edits:        []workspacechange.AppliedEdit{{ID: "edit-1", Hunks: []workspacechange.Hunk{{ID: "one"}, {ID: "two"}}}},
			ReviewStatus: workspacechange.ReviewStatusPending, ApplyState: workspacechange.ApplyStateApplied,
		},
	}
	adapter, err := newWorkspaceMutationAdapter(service, func(context.Context) workspacechange.ChangeMetadata {
		return workspacechange.ChangeMetadata{Origin: workspacechange.OriginAgent, ChangeGroupID: "run-1", RunID: "run-1"}
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Edit(context.Background(), agenttools.EditRequest{
		Path: "chapters/ch01.md", OldString: "old", NewString: "new", ReplaceAll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.readCalls != 1 || service.readPath != "chapters/ch01.md" || service.applyRequest.BaseRevision != "sha256:before" {
		t.Fatalf("unexpected revision lookup/request: %#v", service.applyRequest)
	}
	if len(service.applyRequest.Edits) != 1 || service.applyRequest.Edits[0].OldString != "old" ||
		service.applyRequest.Edits[0].NewString != "new" || !service.applyRequest.Edits[0].ReplaceAll {
		t.Fatalf("exact edit was not preserved: %#v", service.applyRequest.Edits)
	}
	if service.applyRequest.Metadata.RunID != "run-1" {
		t.Fatalf("metadata = %#v", service.applyRequest.Metadata)
	}
	if !strings.Contains(result.ModelContent, `"change_set_id":"change-1"`) ||
		!strings.Contains(result.ModelContent, `"replacements":2`) ||
		strings.Contains(result.ModelContent, "secret-before") || len(result.ModelContent) > 4096 {
		t.Fatalf("receipt = %s", result.ModelContent)
	}
}

func TestWorkspaceEditUsesCurrentContentAfterEarlierExternalChange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "ideas.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// This simulates a user edit after the Agent's earlier read but before the
	// edit tool call. The Adapter deliberately resolves the revision now.
	if err := os.WriteFile(path, []byte("unrelated\nmanual update"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := newWorkspaceMutationAdapter(service)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Edit(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","old_string":"manual update","new_string":"agent update"}`); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "unrelated\nagent update" {
		t.Fatalf("edit did not preserve unrelated current content: %q", content)
	}
}

func TestWorkspaceEditRejectsAmbiguousAndNoOpWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "ideas.md")
	if err := os.WriteFile(path, []byte("same same"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := newWorkspaceMutationAdapter(service)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Edit(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","old_string":"same","new_string":"new"}`); err == nil {
		t.Fatal("ambiguous edit succeeded")
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","old_string":"same","new_string":"same"}`); err == nil {
		t.Fatal("no-op edit succeeded")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "same same" {
		t.Fatalf("failed edit mutated file: %q err=%v", content, err)
	}
}

func TestWorkspaceWriteDerivesCurrentOrMissingRevisionInternally(t *testing.T) {
	service := &recordingWorkspaceChangeService{
		workspace: t.TempDir(), readRevision: "sha256:current",
		changeSet: workspacechange.ChangeSet{ID: "write-1", GroupID: "group-1", Path: "ideas.md"},
	}
	adapter, err := newWorkspaceMutationAdapter(service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Write(context.Background(), agenttools.WriteRequest{Path: "ideas.md", Content: "new"}); err != nil {
		t.Fatal(err)
	}
	if service.replaceRequest.BaseRevision != "sha256:current" || service.replaceRequest.Content != "new" {
		t.Fatalf("replace request = %#v", service.replaceRequest)
	}

	service.readErr = &workspacechange.Error{Code: workspacechange.ErrorCodeNotFound, Message: "not found"}
	service.changeSet = workspacechange.ChangeSet{ID: "write-2", GroupID: "group-2", Path: "new.md"}
	if _, err := adapter.Write(context.Background(), agenttools.WriteRequest{Path: "new.md", Content: "created"}); err != nil {
		t.Fatal(err)
	}
	if service.replaceRequest.BaseRevision != "missing" {
		t.Fatalf("missing revision = %#v", service.replaceRequest)
	}
}

func TestWorkspaceChangeMetadataUsesNativeToolIdentity(t *testing.T) {
	ctx := agent.ContextWithToolCall(context.Background(), "call-native", "edit")
	metadata := workspaceChangeMetadata(ctx, nil)
	if metadata.ToolCallID != "call-native" || metadata.ChangeGroupID != "call-native" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkspaceChangeErrorIsStructuredAndHidesRevisions(t *testing.T) {
	err := &workspacechange.Error{
		Code: workspacechange.ErrorCodeRevisionConflict, Message: "workspace file revision changed",
		Details: map[string]any{
			"path": "chapters/ch01.md", "expected_revision": "sha256:before",
			"actual_revision": "sha256:after", "workspace_mutated": false,
		},
	}
	message, ok := formatWorkspaceChangeToolError("edit", err)
	if !ok || !strings.Contains(message, `"code":"revision_conflict"`) ||
		!strings.Contains(message, `"retryable":true`) ||
		strings.Contains(message, "expected_revision") || strings.Contains(message, "sha256:") {
		t.Fatalf("structured error = %s", message)
	}
}

func TestWorkspaceChangeReceiptTrustsOnlyNewMutationToolNames(t *testing.T) {
	content := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	for _, toolName := range []string{"read", "grep", "read_file", "write_file", "edit_file", "execute"} {
		if _, ok := parseWorkspaceChangeToolReceipt(toolName, content); ok {
			t.Fatalf("untrusted tool %q forged a workspace change receipt", toolName)
		}
	}
	for _, toolName := range []string{"write", "edit"} {
		receipt, ok := parseWorkspaceChangeToolReceipt(toolName, content)
		if !ok || receipt.ChangeSetID != "change-1" {
			t.Fatalf("receipt for %s = %#v ok=%t", toolName, receipt, ok)
		}
	}
}
