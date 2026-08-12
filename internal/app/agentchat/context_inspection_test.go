package agentchat

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	chatagent "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
)

func TestAgentChatAnalyzeContextUsesPublicSessionInspection(t *testing.T) {
	service, binding, store := newInjectedService(t, nil, "inspect-agent-chat")
	sess, err := store.GetOrCreate(binding.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("AGENT_CHAT_PRIOR_CONTEXT")); err != nil {
		t.Fatal(err)
	}
	executionRuntime := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = executionRuntime.Close(context.Background()) })
	runtime := service.projects[binding.ProjectID]
	runtime.executionRuntime = executionRuntime
	runtime.cfg = config.Config{
		Workspace: runtime.workspace, ProjectID: runtime.projectID, ProjectStateDir: runtime.stateRoot,
		NovaDir: t.TempDir(), OpenAIBaseURL: "https://example.invalid", OpenAIModel: "inspection-model",
	}

	analysis, err := service.AnalyzeContext(context.Background(), binding, chatagent.ChatRequest{
		Message: "AGENT_CHAT_CURRENT_CONTEXT",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := contextAnalysisContent(analysis.ContextMessages)
	for _, want := range []string{"AGENT_CHAT_PRIOR_CONTEXT", "AGENT_CHAT_CURRENT_CONTEXT"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("AgentChat public inspection is missing %q:\n%s", want, joined)
		}
	}
	if analysis.AgentKind != binding.agentKind || analysis.Mode != "ide" || analysis.SystemPrompt == "" {
		t.Fatalf("AgentChat inspection metadata = %#v", analysis)
	}
}

func contextAnalysisContent(parts []chatagent.ContextAnalysisPart) string {
	var joined strings.Builder
	for _, part := range parts {
		joined.WriteString(part.Content)
		joined.WriteByte('\n')
	}
	return joined.String()
}
