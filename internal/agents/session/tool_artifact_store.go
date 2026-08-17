package session

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolartifact"
)

const sessionToolArtifactDirectorySuffix = ".artifacts"

// ToolArtifactStore returns the filesystem adapter bound to this canonical
// session. Artifact lifetime follows the session journal lifetime. The shared
// store implementation applies the same boundary and publication rules used
// by writing and game conversations.
func (s *Session) ToolArtifactStore() agent.ToolArtifactBackend {
	if s == nil || strings.TrimSpace(s.filePath) == "" {
		return nil
	}
	store, err := toolartifact.NewBoundedStore(
		filepath.Dir(s.filePath),
		sessionToolArtifactDirectory(s.filePath),
	)
	if err != nil {
		return failedToolArtifactStore{err: err}
	}
	return store
}

type failedToolArtifactStore struct{ err error }

func (store failedToolArtifactStore) BeginToolArtifact(context.Context, agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	return nil, fmt.Errorf("initialize session tool artifact store: %w", store.err)
}

func (store failedToolArtifactStore) VerifyToolArtifact(context.Context, agent.ToolArtifactRef, agent.ToolArtifactRequest) error {
	return fmt.Errorf("initialize session tool artifact store: %w", store.err)
}

func sessionToolArtifactDirectory(journalPath string) string {
	return filepath.Clean(journalPath) + sessionToolArtifactDirectorySuffix
}
