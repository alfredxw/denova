package change

import (
	"strings"
	"testing"
)

func TestFinalToolReceiptPersistsOnlyProjectRelativeMutationIdentity(t *testing.T) {
	content, err := MarshalToolReceipt(ChangeSet{
		ID: "change-one", GroupID: "group-one", Path: "chapters/one.md",
		BaseRevision: "sha256:before", Revision: "sha256:after",
		ReviewStatus: "pending", ApplyState: ApplyStateApplied,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, `"workspace"`) {
		t.Fatalf("final tool receipt retained runtime workspace: %s", content)
	}
	receipt, ok := ParseToolReceipt("write", content)
	if !ok || receipt.Workspace != "" || receipt.Path != "chapters/one.md" {
		t.Fatalf("portable tool receipt = %#v ok=%t", receipt, ok)
	}
}

func TestFinalToolReceiptStillReadsReleasedWorkspaceReceipt(t *testing.T) {
	legacy := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"C:\\old\\book","change_group_id":"group-one","change_set_id":"change-one","path":"chapters/one.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	receipt, ok := ParseToolReceipt("write", legacy)
	if !ok || receipt.Workspace != `C:\old\book` {
		t.Fatalf("released tool receipt = %#v ok=%t", receipt, ok)
	}
}
