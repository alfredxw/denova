package versions

import (
	"sync"
	"time"
)

const defaultAutoVersionIdleDelay = 30 * time.Second

// Service 管理当前书籍 workspace 的 go-git 本地版本库。
type Service struct {
	workspace string
	mu        sync.Mutex

	autoMu               sync.Mutex
	autoTimer            *time.Timer
	autoGeneration       uint64
	autoRunning          bool
	autoClosed           bool
	autoSettings         VersionAutoSettings
	autoVersionIdleDelay time.Duration
}

func NewService(workspace string) *Service {
	return &Service{
		workspace:            workspace,
		autoVersionIdleDelay: defaultAutoVersionIdleDelay,
	}
}

func DefaultAutoSettings() VersionAutoSettings {
	return VersionAutoSettings{
		TimedEnabled:         true,
		TimedIntervalMinutes: DefaultTimedVersionIntervalMinutes,
		Retention:            DefaultAutoVersionRetention,
	}
}
