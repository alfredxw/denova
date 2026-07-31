package config

import "testing"

func TestAgentApprovalModeUsesWriteAsProductDefault(t *testing.T) {
	if mode := DefaultSettings().AgentApprovalMode; mode != AgentApprovalWrite {
		t.Fatalf("default Agent approval mode = %q, want %q", mode, AgentApprovalWrite)
	}
}

func TestAgentApprovalModeRejectsUnknownValues(t *testing.T) {
	if mode := NormalizeAgentApprovalMode("unknown"); mode != AgentApprovalAsk {
		t.Fatalf("normalized unknown mode = %q, want fail-closed %q", mode, AgentApprovalAsk)
	}
	if _, err := ParseAgentApprovalMode("unknown"); err == nil {
		t.Fatal("unknown mode was accepted")
	}
}
