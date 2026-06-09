package versions

import (
	"path/filepath"
	"sync"
)

// Service 管理当前书籍 workspace 的 go-git 本地版本库。
type Service struct {
	workspace string
	mu        sync.Mutex
}

func NewService(workspace string) *Service {
	return &Service{workspace: workspace}
}

func DefaultAutoSettings() VersionAutoSettings {
	return VersionAutoSettings{
		TimedEnabled:         true,
		TimedIntervalMinutes: DefaultTimedVersionIntervalMinutes,
		AgentEnabled:         true,
		AgentCharThreshold:   DefaultAgentVersionCharThreshold,
		Retention:            DefaultAutoVersionRetention,
	}
}

func (s *Service) versionsDir() string {
	return filepath.Join(s.workspace, ".nova", "versions")
}

func (s *Service) repoDir() string {
	return filepath.Join(s.versionsDir(), "git")
}

func (s *Service) indexPath() string {
	return filepath.Join(s.versionsDir(), "index.json")
}
