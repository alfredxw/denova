package versions

import "sync"

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
