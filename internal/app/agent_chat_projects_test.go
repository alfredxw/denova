package app

import (
	"fmt"
	"testing"
	"time"

	"denova/internal/agents/session"
)

func TestVisibleAgentChatProjectSessionsKeepsRunningOutsideRecentWindow(t *testing.T) {
	metas := make([]session.SessionMeta, AgentChatProjectSessionsLimit+2)
	for index := range metas {
		metas[index] = session.SessionMeta{ID: fmt.Sprintf("session-%d", index), UpdatedAt: time.Unix(int64(index), 0)}
	}
	workspace := "/books/alpha"
	detached := metas[len(metas)-1].ID
	running := map[string]struct{}{
		agentChatBindingKey(AgentChatBinding{Workspace: workspace, SessionID: detached}): {},
	}

	visible := visibleAgentChatProjectSessions(metas, workspace, running)
	if len(visible) != AgentChatProjectSessionsLimit+1 {
		t.Fatalf("visible session count = %d, want %d", len(visible), AgentChatProjectSessionsLimit+1)
	}
	if visible[len(visible)-1].ID != detached {
		t.Fatalf("last visible session = %q, want detached running session %q", visible[len(visible)-1].ID, detached)
	}
}
