package tools

import (
	"fmt"
	"strings"

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
	environment []string,
	projectStateRoot string,
) (agenttools.CommandRunner, error) {
	if workspace == nil || workspace.Root() == "" {
		return nil, fmt.Errorf("shell workspace is required")
	}
	var changes *workspacechange.Service
	var err error
	if strings.TrimSpace(projectStateRoot) != "" {
		changes, err = workspacechange.ForWorkspaceAt(workspace.Root(), projectStateRoot)
	} else {
		changes, err = workspacechange.ForWorkspace(workspace.Root())
	}
	if err != nil {
		return nil, fmt.Errorf("coordinate shell workspace: %w", err)
	}
	return agenttools.NewLocalCommandRunner(agenttools.CommandRunnerOptions{
		Workspace:       workspace,
		Shell:           shell,
		Executable:      executable,
		BaseEnvironment: append([]string(nil), environment...),
		Guard:           changes.WithExclusiveWorkspace,
	})
}
