package tools

import (
	"fmt"

	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/internal/workspacechange"
)

// newAgentCommandRunner binds the reusable Process implementation to Denova's
// workspace mutation coordinator, so shell effects cannot race editor saves,
// reviewed changes, undo, or rewind operations.
func newAgentCommandRunner(
	workspace *agenttools.LocalWorkspace,
	shell agenttools.ShellKind,
	executable string,
) (agenttools.CommandRunner, error) {
	if workspace == nil || workspace.Root() == "" {
		return nil, fmt.Errorf("shell workspace is required")
	}
	changes, err := workspacechange.ForWorkspace(workspace.Root())
	if err != nil {
		return nil, fmt.Errorf("coordinate shell workspace: %w", err)
	}
	return agenttools.NewLocalCommandRunner(agenttools.CommandRunnerOptions{
		Workspace:  workspace,
		Shell:      shell,
		Executable: executable,
		Guard:      changes.WithExclusiveWorkspace,
	})
}
