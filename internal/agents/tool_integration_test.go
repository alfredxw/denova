package agents

import (
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/illustration"
)

func TestGeneratedImageToolResultTracksMutationTarget(t *testing.T) {
	payload := illustration.Result{
		Schema:   illustration.ResultSchema,
		MetaPath: "assets/illustrations/ch01/run/meta.json",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := producttools.WorkspaceWriteDescriptor(producttools.ToolSourceImage, config.AgentToolImageGeneration, agent.ToolRecoveryNonIdempotent)
	record := ToolExecutionRecord{
		ToolName: producttools.GenerateImageToolName, ExecutionID: "call-image", Status: "success", Descriptor: descriptor,
	}
	applyToolMutationReceiptToExecutionRecord(&record, agent.TextToolResult(string(raw)))
	mutation, ok := toolMutationFromExecutionRecord(record)
	if !ok || mutation.Source != ToolSourceImage || mutation.Target != payload.MetaPath || mutation.PostCheck != ToolPostCheckWorkspaceChange {
		t.Fatalf("unexpected mutation: %#v, committed=%t record=%#v", mutation, ok, record)
	}
}

func TestWorkspaceChangeReceiptHidesInternalRevisionsFromModel(t *testing.T) {
	raw := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	filtered := FilterToolResultForModel("edit", `{"path":"chapters/ch01.md","edits":[]}`, raw)
	if !strings.Contains(filtered.Content, `"change_set_id":"change-1"`) {
		t.Fatalf("model receipt lost public change identity: %s", filtered.Content)
	}
	if strings.Contains(filtered.Content, "base_revision") || strings.Contains(filtered.Content, `"revision"`) || strings.Contains(filtered.Content, "sha256:") {
		t.Fatalf("model receipt exposed internal revisions: %s", filtered.Content)
	}
}

func TestToolExecutionRecordAssociatesWorkspaceChangeReceipt(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	descriptor := producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)
	record := ToolExecutionRecord{ToolName: "edit", ExecutionID: "call-1", Status: "success", Descriptor: descriptor}
	applyToolMutationReceiptToExecutionRecord(&record, agent.ToolResult{Details: []byte(receipt)})
	mutation, ok := toolMutationFromExecutionRecord(record)
	if !ok {
		t.Fatalf("record did not produce a mutation: %#v", record)
	}
	if mutation.Workspace != "/workspace/book-a" || mutation.ChangeGroupID != "group-1" || mutation.ChangeSetID != "change-1" || mutation.Revision != "sha256:after" || mutation.Target != "chapters/ch01.md" {
		t.Fatalf("workspace change identity was not tracked: %#v", mutation)
	}
}

func TestWorkspaceChangeReceiptUpdatesOnlyTrustedToolExecutionRecords(t *testing.T) {
	content := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	record := ToolExecutionRecord{ToolName: "write"}
	applyWorkspaceChangeReceiptToExecutionRecord(&record, agent.ToolResult{Details: []byte(content)})
	if record.Workspace != "/workspace/book-a" || record.ChangeSetID != "change-1" {
		t.Fatalf("execution record lost workspace identity: %#v", record)
	}
	forged := ToolExecutionRecord{ToolName: "read"}
	applyWorkspaceChangeReceiptToExecutionRecord(&forged, agent.ToolResult{Details: []byte(content)})
	if forged.Workspace != "" || forged.ChangeSetID != "" {
		t.Fatalf("read forged an execution record receipt: %#v", forged)
	}
}
