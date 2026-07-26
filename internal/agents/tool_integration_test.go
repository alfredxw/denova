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
	tracker := newMutationTracker()
	descriptor := producttools.WorkspaceWriteDescriptor(producttools.ToolSourceImage, config.AgentToolImageGeneration, agent.ToolRecoveryNonIdempotent)
	filtered := filterToolResultForModelWithDescriptor(
		producttools.GenerateImageToolName,
		descriptor,
		`{"purpose":"chapter_illustration","target_path":"chapters/ch01.md","prompt":"雨夜"}`,
		string(raw),
		0,
	)
	tracker.Observe(Event{Type: "tool_call", Data: map[string]any{
		"id":             "call-image",
		"name":           producttools.GenerateImageToolName,
		"args":           `{"purpose":"chapter_illustration","target_path":"chapters/ch01.md","prompt":"雨夜"}`,
		"source":         string(descriptor.Source),
		"mutation_scope": string(descriptor.MutationScope),
		"post_check":     string(descriptor.PostCheck),
	}})
	tracker.Observe(Event{Type: "tool_result", Data: map[string]any{
		"id":      "call-image",
		"name":    producttools.GenerateImageToolName,
		"content": filtered.Content,
		"target":  payload.MetaPath,
	}})
	mutations := tracker.Mutations()
	if len(mutations) != 1 || mutations[0].Source != ToolSourceImage || mutations[0].Target != payload.MetaPath || mutations[0].PostCheck != ToolPostCheckWorkspaceChange {
		t.Fatalf("unexpected mutations: %#v", mutations)
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

func TestMutationTrackerAssociatesWorkspaceChangeReceipt(t *testing.T) {
	tracker := newMutationTracker()
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	descriptor := producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)
	tracker.Observe(Event{Type: "tool_call", Data: map[string]any{
		"id":             "call-1",
		"name":           "edit",
		"args":           `{"path":"chapters/ch01.md","edits":[]}`,
		"source":         string(descriptor.Source),
		"mutation_scope": string(descriptor.MutationScope),
		"post_check":     string(descriptor.PostCheck),
	}})
	tracker.Observe(Event{Type: "tool_result", Data: map[string]any{
		"id": "call-1", "name": "edit", "content": receipt,
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
