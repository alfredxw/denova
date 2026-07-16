package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

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

func (s *recordingWorkspaceChangeService) Workspace() string {
	return s.workspace
}

func (s *recordingWorkspaceChangeService) ReadFile(path string) (string, string, error) {
	s.readCalls++
	s.readPath = path
	if s.readErr != nil {
		return "", "", s.readErr
	}
	revision := s.readRevision
	if revision == "" {
		revision = "sha256:current"
	}
	return "", revision, nil
}

func (s *recordingWorkspaceChangeService) ApplyEdits(_ context.Context, request workspacechange.ApplyEditsRequest) (workspacechange.ChangeSet, error) {
	s.applyCalls++
	s.applyRequest = request
	return s.changeSet, s.err
}

func (s *recordingWorkspaceChangeService) ReplaceFile(_ context.Context, request workspacechange.ReplaceFileRequest) (workspacechange.ChangeSet, error) {
	s.replaceCalls++
	s.replaceRequest = request
	return s.changeSet, s.err
}

func TestWorkspaceEditFileToolBatchesOneFileAndReturnsBoundedReceipt(t *testing.T) {
	service := &recordingWorkspaceChangeService{workspace: t.TempDir(), readRevision: "sha256:before", changeSet: workspacechange.ChangeSet{
		ID:            "change-1",
		GroupID:       "run-1",
		Path:          "chapters/ch01.md",
		BaseRevision:  "sha256:before",
		Revision:      "sha256:after",
		BeforeContent: strings.Repeat("before", 100),
		AfterContent:  strings.Repeat("after", 100),
		ReviewStatus:  workspacechange.ReviewStatusPending,
		ApplyState:    workspacechange.ApplyStateApplied,
		Edits: []workspacechange.AppliedEdit{
			{ID: "opening", Hunks: []workspacechange.Hunk{{ID: "h1"}}},
			{ID: "ending", Hunks: []workspacechange.Hunk{{ID: "h2"}, {ID: "h3"}}},
		},
	}}
	for index := 0; index < 10_000; index++ {
		service.changeSet.Edits = append(service.changeSet.Edits, workspacechange.AppliedEdit{
			ID:    fmt.Sprintf("bulk-%d", index),
			Hunks: []workspacechange.Hunk{{ID: fmt.Sprintf("bulk-hunk-%d", index)}},
		})
	}
	base, err := newWorkspaceEditFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	invokable, ok := base.(tool.InvokableTool)
	if !ok {
		t.Fatal("edit_file should be invokable")
	}
	observer := newRunObserver(&RunLedger{id: "run-1"}, "")
	ctx := ContextWithRunObserver(context.Background(), observer)
	result, err := invokable.InvokableRun(ctx, `{
        "file_path":"chapters/ch01.md",
        "edits":[
          {"id":"opening","old_string":"old 1","new_string":"new 1"},
          {"id":"ending","old_string":"old 2","new_string":"new 2","replace_all":true}
        ]
      }`)
	if err != nil {
		t.Fatal(err)
	}
	if service.readCalls != 1 || service.readPath != "chapters/ch01.md" || service.applyRequest.Path != "chapters/ch01.md" || service.applyRequest.BaseRevision != "sha256:before" {
		t.Fatalf("unexpected request: %#v", service.applyRequest)
	}
	if len(service.applyRequest.Edits) != 2 || !service.applyRequest.Edits[1].ReplaceAll {
		t.Fatalf("batch edits were not preserved: %#v", service.applyRequest.Edits)
	}
	if service.applyRequest.Metadata.Origin != workspacechange.OriginAgent ||
		service.applyRequest.Metadata.RunID != "run-1" ||
		service.applyRequest.Metadata.ChangeGroupID != "run-1" {
		t.Fatalf("unexpected metadata: %#v", service.applyRequest.Metadata)
	}
	var receipt workspaceChangeToolReceipt
	if err := json.Unmarshal([]byte(result), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Schema != workspaceChangeToolResultSchema || receipt.Workspace != service.workspace || receipt.ChangeSetID != "change-1" || len(receipt.Edits) != 0 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if strings.Contains(result, `"edits"`) || len(result) > 4096 {
		t.Fatalf("receipt grew with per-edit details: bytes=%d result=%s", len(result), result)
	}
	if strings.Contains(result, "beforebefore") || strings.Contains(result, "afterafter") {
		t.Fatalf("receipt leaked file content: %s", result)
	}
}

func TestWorkspaceChangeMetadataUsesStableRunIdentityWithoutLedger(t *testing.T) {
	observer := newRunObserverWithIdentity(nil, "", "task-run", "session-1", "review-thread-1")
	metadata := workspaceChangeMetadata(ContextWithRunObserver(context.Background(), observer))

	if metadata.ChangeGroupID != "task-run" || metadata.RunID != "task-run" {
		t.Fatalf("stable run identity was lost without a ledger: %#v", metadata)
	}
	if metadata.SessionID != "session-1" || metadata.ReviewThreadID != "review-thread-1" {
		t.Fatalf("review linkage metadata was lost: %#v", metadata)
	}
}

func TestWorkspaceEditFileToolPublishesBatchSchema(t *testing.T) {
	base, err := newWorkspaceEditFileTool(&recordingWorkspaceChangeService{workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	info, err := base.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		JSONSchema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"json_schema"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file_path", "edits"} {
		if _, ok := encoded.JSONSchema.Properties[name]; !ok {
			t.Fatalf("batch edit schema is missing root property %q: %s", name, data)
		}
	}
	if _, ok := encoded.JSONSchema.Properties["base_revision"]; ok || containsStringValue(encoded.JSONSchema.Required, "base_revision") {
		t.Fatalf("batch edit schema exposes internal base_revision: %s", data)
	}
	for _, legacy := range []string{"old_string", "new_string", "replace_all"} {
		if _, ok := encoded.JSONSchema.Properties[legacy]; ok {
			t.Fatalf("legacy single-edit property %q remains at schema root: %s", legacy, data)
		}
	}
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestWorkspaceWriteFileToolUsesChangeService(t *testing.T) {
	service := &recordingWorkspaceChangeService{workspace: t.TempDir(), readRevision: "sha256:before", changeSet: workspacechange.ChangeSet{
		ID:           "change-write",
		GroupID:      "group-write",
		Path:         "ideas.md",
		BaseRevision: "sha256:before",
		Revision:     "sha256:after",
		ReviewStatus: workspacechange.ReviewStatusPending,
		ApplyState:   workspacechange.ApplyStateApplied,
	}}
	base, err := newWorkspaceWriteFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	result, err := base.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"ideas.md","content":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	if service.readCalls != 1 || service.readPath != "ideas.md" || service.replaceRequest.Path != "ideas.md" || service.replaceRequest.Content != "new" || service.replaceRequest.BaseRevision != "sha256:before" {
		t.Fatalf("unexpected replace request: %#v", service.replaceRequest)
	}
	if !strings.Contains(result, `"change_set_id":"change-write"`) {
		t.Fatalf("unexpected write receipt: %s", result)
	}
}

func TestWorkspaceWriteFileToolHidesBaseRevisionFromSchema(t *testing.T) {
	base, err := newWorkspaceWriteFileTool(&recordingWorkspaceChangeService{workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	info, err := base.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		JSONSchema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"json_schema"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := encoded.JSONSchema.Properties["base_revision"]; ok || containsStringValue(encoded.JSONSchema.Required, "base_revision") {
		t.Fatalf("write schema exposes internal base_revision: %s", data)
	}
}

func TestWorkspaceEditFileToolLeavesFileUntouchedWhenOneBatchEditFails(t *testing.T) {
	workspace := t.TempDir()
	chapterDir := filepath.Join(workspace, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(chapterDir, "ch01.md")
	original := "opening\n\nmiddle\n\nending"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceEditFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(tool.InvokableTool).InvokableRun(context.Background(), `{
	        "file_path":"chapters/ch01.md",
	        "edits":[
          {"id":"valid","old_string":"opening","new_string":"new opening"},
          {"id":"missing","old_string":"not present","new_string":"replacement"}
        ]
      }`)
	if err == nil {
		t.Fatal("batch with a missing anchor should fail")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != original {
		t.Fatalf("failed batch changed file: %q", content)
	}
	groups, listErr := service.ListGroups(context.Background(), workspacechange.ChangeFilter{})
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(groups) != 0 {
		t.Fatalf("failed batch created review history: %#v", groups)
	}
}

func TestWorkspaceChangeErrorBecomesStructuredToolError(t *testing.T) {
	err := &workspacechange.Error{
		Code:    workspacechange.ErrorCodeInvalidEdit,
		Message: "old_string appears more than once",
		Details: map[string]any{"edit_index": 1, "match_count": 2},
	}
	message, ok := formatWorkspaceChangeToolError("edit_file", err)
	if !ok {
		t.Fatal("workspace change error should be recognized")
	}
	if !strings.HasPrefix(message, "[tool error]\n") ||
		!strings.Contains(message, `"code":"invalid_edit"`) ||
		!strings.Contains(message, `"workspace_mutated":false`) ||
		!strings.Contains(message, `"retryable":true`) {
		t.Fatalf("unexpected structured error: %s", message)
	}
}

func TestDurabilityPendingToolErrorIsRetryableAndReportsVisibleMutation(t *testing.T) {
	err := &workspacechange.Error{
		Code:    workspacechange.ErrorCodeDurabilityPending,
		Message: "workspace mutation durability is pending",
		Details: map[string]any{"path": "chapters/ch01.md", "workspace_mutated": true},
	}
	message, ok := formatWorkspaceChangeToolError("edit_file", err)
	if !ok ||
		!strings.Contains(message, `"code":"durability_pending"`) ||
		!strings.Contains(message, `"workspace_mutated":true`) ||
		!strings.Contains(message, `"retryable":true`) {
		t.Fatalf("unexpected durability receipt: %s", message)
	}
}

func TestWorkspaceFileToolsResolveCurrentRevisionWithoutModelInput(t *testing.T) {
	service := &recordingWorkspaceChangeService{workspace: t.TempDir(), readRevision: "sha256:current"}
	edit, err := newWorkspaceEditFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edit.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"ideas.md","edits":[{"old_string":"a","new_string":"b"}]}`); err != nil {
		t.Fatal(err)
	}
	if service.applyCalls != 1 || service.applyRequest.BaseRevision != "sha256:current" {
		t.Fatalf("edit_file did not resolve the current revision: %#v", service.applyRequest)
	}

	write, err := newWorkspaceWriteFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"ideas.md","content":"new"}`); err != nil {
		t.Fatal(err)
	}
	if service.replaceCalls != 1 || service.replaceRequest.BaseRevision != "sha256:current" || service.readCalls != 2 {
		t.Fatalf("write_file did not resolve the current revision: %#v", service.replaceRequest)
	}
}

func TestWorkspaceWriteFileToolDetectsMissingFileInternally(t *testing.T) {
	service := &recordingWorkspaceChangeService{workspace: t.TempDir(), readErr: &workspacechange.Error{
		Code: workspacechange.ErrorCodeNotFound, Message: "workspace file not found",
	}, changeSet: workspacechange.ChangeSet{
		ID: "created", GroupID: "group", Path: "new.md", BaseRevision: "missing", Revision: "sha256:after",
	}}
	write, err := newWorkspaceWriteFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"new.md","content":"new"}`); err != nil {
		t.Fatal(err)
	}
	if service.replaceCalls != 1 || service.replaceRequest.BaseRevision != "missing" {
		t.Fatalf("create did not derive missing CAS internally: %#v", service.replaceRequest)
	}
}

func TestWorkspaceChangeErrorHidesInternalRevisionsFromAgent(t *testing.T) {
	err := &workspacechange.Error{
		Code:    workspacechange.ErrorCodeRevisionConflict,
		Message: "workspace file revision changed",
		Details: map[string]any{
			"path":              "chapters/ch01.md",
			"expected_revision": "sha256:before",
			"actual_revision":   "sha256:after",
		},
	}
	message, ok := formatWorkspaceChangeToolError("edit_file", err)
	if !ok {
		t.Fatal("workspace change error should be recognized")
	}
	if strings.Contains(message, "expected_revision") || strings.Contains(message, "actual_revision") || strings.Contains(message, "sha256:") {
		t.Fatalf("tool error exposed internal revisions: %s", message)
	}
}

func TestWorkspaceChangeReceiptHidesInternalRevisionsFromModel(t *testing.T) {
	raw := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	filtered := FilterToolResultForModel("edit_file", `{"file_path":"chapters/ch01.md","edits":[]}`, raw)
	if !strings.Contains(filtered.Content, `"change_set_id":"change-1"`) {
		t.Fatalf("model receipt lost public change identity: %s", filtered.Content)
	}
	if strings.Contains(filtered.Content, "base_revision") || strings.Contains(filtered.Content, `"revision"`) || strings.Contains(filtered.Content, "sha256:") {
		t.Fatalf("model receipt exposed internal revisions: %s", filtered.Content)
	}
}

func TestMutationTrackerAssociatesWorkspaceChangeReceipt(t *testing.T) {
	tracker := newMutationTracker()
	tracker.Observe(Event{Type: "tool_call", Data: map[string]any{
		"id":   "call-1",
		"name": "edit_file",
		"args": `{"file_path":"chapters/ch01.md","edits":[]}`,
	}})
	tracker.Observe(Event{Type: "tool_result", Data: map[string]any{
		"id":      "call-1",
		"name":    "edit_file",
		"content": `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`,
	}})
	mutations := tracker.Mutations()
	if len(mutations) != 1 {
		t.Fatalf("mutations = %#v", mutations)
	}
	mutation := mutations[0]
	if mutation.Workspace != "/workspace/book-a" || mutation.ChangeGroupID != "group-1" || mutation.ChangeSetID != "change-1" || mutation.Revision != "sha256:after" || mutation.Target != "chapters/ch01.md" {
		t.Fatalf("workspace change identity was not tracked: %#v", mutation)
	}
}

func TestWorkspaceChangeReceiptIsTrustedOnlyForWorkspaceFileTools(t *testing.T) {
	content := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	for _, toolName := range []string{"read_file", "execute", "grep"} {
		if _, ok := parseWorkspaceChangeToolReceipt(toolName, content); ok {
			t.Fatalf("untrusted tool %q forged a workspace change receipt", toolName)
		}
	}
	receipt, ok := parseWorkspaceChangeToolReceipt("edit_file", content)
	if !ok || receipt.Workspace != "/workspace/book-a" || receipt.ChangeSetID != "change-1" {
		t.Fatalf("trusted receipt was not parsed: %#v ok=%t", receipt, ok)
	}
	record := ToolExecutionRecord{ToolName: "write_file"}
	applyWorkspaceChangeReceiptToExecutionRecord(&record, content)
	if record.Workspace != "/workspace/book-a" || record.ChangeSetID != "change-1" {
		t.Fatalf("execution record lost workspace identity: %#v", record)
	}
	forged := ToolExecutionRecord{ToolName: "read_file"}
	applyWorkspaceChangeReceiptToExecutionRecord(&forged, content)
	if forged.Workspace != "" || forged.ChangeSetID != "" {
		t.Fatalf("read_file forged an execution record receipt: %#v", forged)
	}
}
