package config

// AgentPluginScope binds plugin calls to the owning Product Session or Story
// branch. It is runtime-only and does not contain plugin configuration.
type AgentPluginScope struct {
	SessionID string
	StoryID   string
	BranchID  string
}
