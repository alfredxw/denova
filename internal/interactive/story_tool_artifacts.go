package interactive

import (
	"denova/internal/agents/toolartifact"
)

const storyToolArtifactNamespace = "game"

// NewStoryToolArtifactStore binds game artifacts to a stable story/branch
// ownership hierarchy. Raw IDs are hashed by toolartifact and never become
// filesystem path components.
func NewStoryToolArtifactStore(workspaceRoot, storyID, branchID string) (*toolartifact.Store, error) {
	scope, err := storyBranchToolArtifactScope(storyID, branchID)
	if err != nil {
		return nil, err
	}
	return toolartifact.NewWorkspaceScopeStore(workspaceRoot, scope)
}

func (s *Store) removeStoryToolArtifacts(storyID string) error {
	scope, err := storyToolArtifactScope(storyID)
	if err != nil {
		return err
	}
	return toolartifact.RemoveWorkspaceScope(s.root, scope)
}

func (s *Store) removeBranchToolArtifacts(storyID, branchID string) error {
	scope, err := storyBranchToolArtifactScope(storyID, branchID)
	if err != nil {
		return err
	}
	return toolartifact.RemoveWorkspaceScope(s.root, scope)
}

func storyToolArtifactScope(storyID string) (toolartifact.WorkspaceScope, error) {
	return toolartifact.NewWorkspaceScope(storyToolArtifactNamespace,
		toolartifact.WorkspaceScopeOwner{Kind: "story", ID: storyID},
	)
}

func storyBranchToolArtifactScope(storyID, branchID string) (toolartifact.WorkspaceScope, error) {
	return toolartifact.NewWorkspaceScope(storyToolArtifactNamespace,
		toolartifact.WorkspaceScopeOwner{Kind: "story", ID: storyID},
		toolartifact.WorkspaceScopeOwner{Kind: "branch", ID: branchID},
	)
}
