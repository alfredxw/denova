package versions

import (
	"sync"
	"time"
)

const defaultAutoVersionIdleDelay = 30 * time.Second

// Service manages one Book Project's local go-git version history.
type Service struct {
	workspace  string
	repository string
	mu         sync.Mutex

	autoMu               sync.Mutex
	autoTimer            *time.Timer
	autoGeneration       uint64
	autoRunning          bool
	autoClosed           bool
	autoSettings         VersionAutoSettings
	autoVersionIdleDelay time.Duration
}

// NewService binds visible Book content to a Project-owned repository. The
// repository must live outside the content directory so version history follows
// stable Project identity across directory relinks and never occupies the
// user's own .git directory.
func NewService(workspace, repository string) *Service {
	return &Service{
		workspace:            workspace,
		repository:           repository,
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
