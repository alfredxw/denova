package automationapp

import (
	"encoding/json"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
)

func TestProjectHostEffectPayloadOmitsRuntimeWorkspace(t *testing.T) {
	runtimeRoot := `C:\moved\denova\projects\portable`
	projectID, workspace, payload := portableAdmittedToolMutation(agenttoolruntime.CommittedToolMutation{
		Binding: agentrun.RuntimeBinding{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-portable",
			Workspace: runtimeRoot, SessionID: "session-one",
		},
		RuntimeOperation: "operation-one",
		RuntimeCycle:     1,
		ToolCallID:       "tool-one",
		Origin: agenttoolruntime.ToolMutationOrigin{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-portable",
			Workspace: runtimeRoot, SessionID: "session-one",
		},
		Mutation: agenttool.Mutation{
			ToolName: "write", ToolCallID: "tool-one", Workspace: runtimeRoot,
			Target: "chapters/one.md",
		},
	})
	if projectID != "project-portable" || workspace != "" {
		t.Fatalf("portable host-effect owner project=%q workspace=%q", projectID, workspace)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), runtimeRoot) || payload.Binding.Workspace != "" || payload.Origin.Workspace != "" || payload.Mutation.Workspace != "" {
		t.Fatalf("host-effect payload retained runtime workspace: %s", raw)
	}
}
