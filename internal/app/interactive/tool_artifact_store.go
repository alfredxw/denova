package interactiveapp

import (
	agents "denova/internal/agents"
	"denova/internal/interactive"
)

// ToolArtifactStore gives game conversations the same workspace-bounded,
// ordinary-read-compatible artifact contract used by writing sessions.
func (c *Conversation) ToolArtifactStore() agents.ToolArtifactBackend {
	if c == nil {
		return nil
	}
	store, err := interactive.NewStoryToolArtifactStore(c.workspace, c.storyID, c.branchID)
	if err != nil {
		// A nil store is deliberate: the shared result processor applies the
		// fail-bounded/protected semantics and never advertises an unreadable path.
		return nil
	}
	return store
}
