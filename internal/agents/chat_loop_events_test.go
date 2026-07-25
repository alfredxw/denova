package agents

import "testing"

func TestCompletedToolKeySeparatesNestedAgentCalls(t *testing.T) {
	root := agentEventMetadata{
		RunID: "run-1", RootAgentName: "DenovaAgent", AgentName: "DenovaAgent",
		RunPath: []string{"DenovaAgent"},
	}
	child := agentEventMetadata{
		RunID: "run-1", RootAgentName: "DenovaAgent", AgentName: "researcher",
		RunPath: []string{"DenovaAgent", "researcher"}, SubAgent: true, SubAgentSessionID: "child-1",
	}
	rootKey := completedToolKey(root, "read_file", "call-1")
	childKey := completedToolKey(child, "read_file", "call-1")
	if rootKey == childKey {
		t.Fatalf("root and child completion keys collided: %q", rootKey)
	}
	if rootKey != completedToolKey(root, "read_file", "call-1") {
		t.Fatal("completion key is not stable")
	}
}
