package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	workspacechange "denova/internal/workspace/change"
)

type recordingWorkspaceChangeService struct {
	workspace      string
	readCalls      int
	readPath       string
	readRevision   string
	readErr        error
	applyCalls     int
	replaceCalls   int
	deleteCalls    int
	applyRequest   workspacechange.ApplyEditsRequest
	replaceRequest workspacechange.ReplaceFileRequest
	deleteRequest  workspacechange.DeleteFileRequest
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

func (service *recordingWorkspaceChangeService) DeleteFile(_ context.Context, request workspacechange.DeleteFileRequest) (workspacechange.ChangeSet, error) {
	service.deleteCalls++
	service.deleteRequest = request
	return service.changeSet, service.err
}

func TestWorkspaceMutationAdapterMapsAtomicEditBatchAndReturnsPerEditReceipt(t *testing.T) {
	service := &recordingWorkspaceChangeService{
		workspace: t.TempDir(), readRevision: "sha256:before",
		changeSet: workspacechange.ChangeSet{
			ID: "change-1", GroupID: "run-1", ReviewThreadID: "review-1",
			Path: "chapters/ch01.md", BaseRevision: "sha256:before", Revision: "sha256:after",
			BeforeContent: strings.Repeat("secret-before", 100), AfterContent: strings.Repeat("secret-after", 100),
			Edits: []workspacechange.AppliedEdit{
				{ID: "edit-1", Hunks: []workspacechange.Hunk{{ID: "one"}, {ID: "two"}}},
				{ID: "edit-2", Hunks: []workspacechange.Hunk{{ID: "three"}}},
			},
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
		Path: "chapters/ch01.md",
		Edits: []agenttools.EditReplacement{
			{OldString: "old", NewString: "new", ReplaceAll: true},
			{OldString: "ending", NewString: "finale"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.readCalls != 1 || service.readPath != "chapters/ch01.md" || service.applyRequest.BaseRevision != "sha256:before" {
		t.Fatalf("unexpected revision lookup/request: %#v", service.applyRequest)
	}
	if service.applyCalls != 1 || len(service.applyRequest.Edits) != 2 ||
		service.applyRequest.Edits[0].OldString != "old" || service.applyRequest.Edits[0].NewString != "new" || !service.applyRequest.Edits[0].ReplaceAll ||
		service.applyRequest.Edits[1].OldString != "ending" || service.applyRequest.Edits[1].NewString != "finale" || service.applyRequest.Edits[1].ReplaceAll {
		t.Fatalf("atomic edit batch was not preserved: %#v", service.applyRequest.Edits)
	}
	if service.applyRequest.Metadata.RunID != "run-1" {
		t.Fatalf("metadata = %#v", service.applyRequest.Metadata)
	}
	receipt, ok := workspacechange.ParseToolReceipt("edit", result.ModelContent)
	if !ok || len(receipt.Edits) != 2 || receipt.Edits[0].ID != "edit-1" || receipt.Edits[0].Replacements != 2 ||
		receipt.Edits[1].ID != "edit-2" || receipt.Edits[1].Replacements != 1 ||
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
	result, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","edits":[{"old_string":"unrelated","new_string":"context"},{"old_string":"manual update","new_string":"agent update"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := workspacechange.ParseToolReceipt("edit", result.ModelContent)
	if !ok || receipt.FileStats == nil {
		t.Fatalf("edit result has no file stats: %s", result.ModelContent)
	}
	if got := *receipt.FileStats; got.Bytes != 20 || got.Characters != 20 ||
		got.NonWhitespaceCharacters != 18 || got.Lines != 2 {
		t.Fatalf("edit file stats = %#v", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "context\nagent update" {
		t.Fatalf("edit did not preserve unrelated current content: %q", content)
	}
}

func TestWorkspaceEditDeleteUsesDurableChangeServiceAndReturnsReviewReceipt(t *testing.T) {
	service := &recordingWorkspaceChangeService{
		workspace: t.TempDir(), readRevision: "sha256:before",
		changeSet: workspacechange.ChangeSet{
			ID: "delete-1", GroupID: "run-delete", ReviewThreadID: "review-delete",
			Path: "chapters/obsolete.md", BaseRevision: "sha256:before", Revision: "missing",
			BeforeExists: true, AfterExists: false,
			Edits:        []workspacechange.AppliedEdit{{ID: "edit-delete", Hunks: []workspacechange.Hunk{{ID: "whole-file"}}}},
			ReviewStatus: workspacechange.ReviewStatusPending, ApplyState: workspacechange.ApplyStateApplied,
		},
	}
	adapter, err := newWorkspaceMutationAdapter(service, func(context.Context) workspacechange.ChangeMetadata {
		return workspacechange.ChangeMetadata{Origin: workspacechange.OriginAgent, ChangeGroupID: "run-delete"}
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Edit(context.Background(), agenttools.EditRequest{
		Path: "chapters/obsolete.md", Operation: agenttools.EditOperationDelete, IgnoredEditCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.readCalls != 1 || service.deleteCalls != 1 || service.applyCalls != 0 ||
		service.deleteRequest.Path != "chapters/obsolete.md" || service.deleteRequest.BaseRevision != "sha256:before" ||
		service.deleteRequest.Metadata.ChangeGroupID != "run-delete" {
		t.Fatalf("delete request = %#v service=%#v", service.deleteRequest, service)
	}
	receipt, ok := workspacechange.ParseToolReceipt("edit", result.ModelContent)
	if !ok || receipt.ChangeSetID != "delete-1" || receipt.Path != "chapters/obsolete.md" || receipt.Revision != "missing" ||
		len(receipt.Warnings) != 1 || receipt.Warnings[0] != ignoredDeleteEditsWarning ||
		!strings.Contains(result.ModelContent, `"status":"applied"`) {
		t.Fatalf("delete receipt = %s", result.ModelContent)
	}
	projected := workspacechange.ToolReceiptForModel("edit", result.ModelContent)
	projectedReceipt, ok := workspacechange.ParseToolReceipt("edit", projected)
	if !ok || len(projectedReceipt.Warnings) != 1 || projectedReceipt.Warnings[0] != ignoredDeleteEditsWarning {
		t.Fatalf("projected delete receipt = %s", projected)
	}
}

func TestWorkspaceEditDeleteAppearsInReviewAndRejectRestoresFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "obsolete.md")
	if err := os.WriteFile(path, []byte("remove me\n"), 0o644); err != nil {
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
	result, err := definition.Tool.Run(context.Background(), `{"path":"obsolete.md","operation":"delete","edits":[{"old_string":"ignored","new_string":"replacement"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("deleted file remains visible: %v", statErr)
	}
	receipt, ok := workspacechange.ParseToolReceipt("edit", result.ModelContent)
	if !ok || len(receipt.Warnings) != 1 || receipt.Warnings[0] != ignoredDeleteEditsWarning {
		t.Fatalf("delete result has no review receipt: %s", result.ModelContent)
	}
	group, err := service.GetGroup(context.Background(), receipt.ChangeGroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.ChangeSets) != 1 || !group.ChangeSets[0].BeforeExists || group.ChangeSets[0].AfterExists ||
		group.ChangeSets[0].BeforeContent != "remove me\n" || group.ChangeSets[0].AfterContent != "" ||
		group.PendingEditCount != 1 {
		t.Fatalf("delete review group = %#v", group)
	}
	if _, err := service.Review(context.Background(), workspacechange.ReviewRequest{
		GroupID: receipt.ChangeGroupID, Decision: workspacechange.ReviewDecisionReject,
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "remove me\n" {
		t.Fatalf("review rejection did not restore file: content=%q err=%v", content, err)
	}
}

func TestWorkspaceWriteReturnsCompleteUnicodeFileStats(t *testing.T) {
	workspace := t.TempDir()
	service, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := newWorkspaceMutationAdapter(service)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Write(adapter)
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), `{"path":"chapter.md","content":"第一行\nsecond line"}`)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := workspacechange.ParseToolReceipt("write", result.ModelContent)
	if !ok || receipt.FileStats == nil {
		t.Fatalf("write result has no file stats: %s", result.ModelContent)
	}
	if got := *receipt.FileStats; got.Bytes != 21 || got.Characters != 15 ||
		got.NonWhitespaceCharacters != 13 || got.Lines != 2 {
		t.Fatalf("write file stats = %#v", got)
	}
}

func TestWorkspaceWriteTreatsIdenticalContentAsSuccessfulNoOp(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "chapter.md")
	if err := os.WriteFile(path, []byte("unchanged chapter\n"), 0o644); err != nil {
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
	definition, err := agenttools.Write(adapter)
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), `{"path":"chapter.md","content":"unchanged chapter\n"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"schema":"workspace_change.tool_noop.v1"`, `"status":"unchanged"`,
		`"path":"chapter.md"`, `"workspace_mutated":false`, "continue without retrying",
	} {
		if !strings.Contains(result.ModelContent, expected) {
			t.Fatalf("no-op receipt missing %q: %s", expected, result.ModelContent)
		}
	}
	if _, ok := workspacechange.ParseToolReceipt("write", result.ModelContent); ok {
		t.Fatalf("no-op write must not be projected as a workspace mutation: %s", result.ModelContent)
	}
	groups, err := service.ListGroups(context.Background(), workspacechange.ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("no-op write created review history: %#v", groups)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "unchanged chapter\n" {
		t.Fatalf("no-op write changed the file: content=%q err=%v", content, err)
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
	if _, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","edits":[{"old_string":"same","new_string":"new"}]}`); err == nil {
		t.Fatal("ambiguous edit succeeded")
	}
	_, validationErr := definition.Tool.Run(context.Background(), `{"path":"ideas.md","edits":[{"old_string":"same","new_string":"new"},{"old_string":"missing","new_string":"present"}]}`)
	message, ok := formatWorkspaceChangeToolError("edit", validationErr)
	if validationErr == nil || !ok || !strings.Contains(message, `"issue_count":2`) ||
		!strings.Contains(message, `"edit_index":0`) || !strings.Contains(message, `"code":"not_unique"`) ||
		!strings.Contains(message, `"edit_index":1`) || !strings.Contains(message, `"code":"not_found"`) ||
		!strings.Contains(message, `"workspace_mutated":false`) {
		t.Fatalf("batch validation error = %v, receipt = %s", validationErr, message)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"ideas.md","edits":[{"old_string":"same","new_string":"same"}]}`); err == nil {
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
		if _, ok := workspacechange.ParseToolReceipt(toolName, content); ok {
			t.Fatalf("untrusted tool %q forged a workspace change receipt", toolName)
		}
	}
	for _, toolName := range []string{"write", "edit"} {
		receipt, ok := workspacechange.ParseToolReceipt(toolName, content)
		if !ok || receipt.ChangeSetID != "change-1" {
			t.Fatalf("receipt for %s = %#v ok=%t", toolName, receipt, ok)
		}
	}
}
