package compaction

import (
	"fmt"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func protectedToolMessage(callID, path string, status agent.ToolResultStatus) *agent.Message {
	return &agent.Message{
		Role: agent.ToolRole, Content: "FULL RAW TOOL BODY MUST NOT ENTER SUMMARY", ToolCallID: callID, ToolName: "write",
		ToolResult: &agent.ToolResultSummary{
			Status: status, ResultRetention: agent.ToolResultProtected,
			ProtectedReceipt: &agent.ToolResultProtectedReceipt{
				SanitizedArguments: fmt.Sprintf(`{"path":%q}`, path),
				Outcome:            `{"changed":true}`,
			},
			Artifacts: []agent.ToolArtifactRef{{
				ID: "private-artifact-id", Purpose: agent.ToolArtifactPurposeCompleteToolOutput,
				ReadablePath: path, ContentType: "text/plain", SHA256: strings.Repeat("a", 64),
				EstimatedBytes: 1024, EstimatedTokens: 256, Complete: true,
			}},
		},
	}
}

func TestCompactionPreservesBoundedProtectedReceiptsWithoutRawArtifactSecrets(t *testing.T) {
	message := protectedToolMessage("call-one", ".agent/artifacts/call-one.txt", agent.ToolResultSuccess)
	summary := mergeProtectedReceiptContext("narrative summary", "", []*agent.Message{message}, 64<<10)
	for _, want := range []string{
		"narrative summary", protectedReceiptTitle, "call-one", ".agent/artifacts/call-one.txt", `\"changed\":true`,
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("protected summary missing %q: %s", want, summary)
		}
	}
	for _, forbidden := range []string{"FULL RAW TOOL BODY", "private-artifact-id", strings.Repeat("a", 64), "sha256"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("protected summary leaked %q: %s", forbidden, summary)
		}
	}
	if len(summary) > 64<<10 {
		t.Fatalf("protected summary bytes=%d", len(summary))
	}
}

func TestRepeatedCompactionMergesProtectedReceiptsWithoutDuplication(t *testing.T) {
	first := protectedToolMessage("call-one", ".agent/artifacts/one.txt", agent.ToolResultSuccess)
	previous := mergeProtectedReceiptContext("first summary", "", []*agent.Message{first}, 64<<10)
	second := protectedToolMessage("call-two", ".agent/artifacts/two.txt", agent.ToolResultError)
	current := mergeProtectedReceiptContext("second summary", previous, []*agent.Message{first, second}, 64<<10)
	if strings.Count(current, `"call_id":"call-one"`) != 1 || strings.Count(current, `"call_id":"call-two"`) != 1 {
		t.Fatalf("repeated protected receipts were duplicated or lost: %s", current)
	}
	_, block := splitProtectedReceipts(current)
	lines := strings.Split(block, "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], `"call_id":"call-two"`) {
		t.Fatalf("unresolved receipt was not retained first: %s", block)
	}
}

func TestProtectedReceiptSelectionIsBoundedAndReportsOmissions(t *testing.T) {
	messages := make([]*agent.Message, 0, protectedReceiptLimit+12)
	for index := 0; index < protectedReceiptLimit+12; index++ {
		status := agent.ToolResultSuccess
		if index == 0 {
			status = agent.ToolResultError
		}
		messages = append(messages, protectedToolMessage(
			fmt.Sprintf("call-%02d", index), fmt.Sprintf(".agent/artifacts/%02d.txt", index), status,
		))
	}
	block := receiptsFromMessages(messages)
	lines := strings.Split(block, "\n")
	if len(lines) > protectedReceiptLimit+1 || len(block) > protectedReceiptBytes {
		t.Fatalf("receipt block lines=%d bytes=%d", len(lines), len(block))
	}
	if !strings.Contains(lines[0], `"call_id":"call-00"`) || !strings.Contains(block, `"omitted_receipts"`) {
		t.Fatalf("bounded receipt selection=%s", block)
	}
}
