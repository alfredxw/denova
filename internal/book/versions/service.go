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
	autoRetired          bool
	autoLeases           int
	autoSettings         VersionAutoSettings
	autoVersionIdleDelay time.Duration
}

// Acquire keeps this workspace version service alive for an already-started
// background run. New leases are rejected after the workspace is retired.
func (s *Service) Acquire() (func(), bool) {
	s.autoMu.Lock()
	if s.autoClosed || s.autoRetired {
		s.autoMu.Unlock()
		return func() {}, false
	}
	s.autoLeases++
	s.autoMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.autoMu.Lock()
			if s.autoLeases > 0 {
				s.autoLeases--
			}
			s.closeRetiredIfIdleLocked()
			s.autoMu.Unlock()
		})
	}, true
}

// Retire rejects new runtime leases while allowing existing runs and their
// already-scheduled automatic version to finish for the source workspace.
func (s *Service) Retire() {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoClosed {
		return
	}
	s.autoRetired = true
	s.closeRetiredIfIdleLocked()
}

func (s *Service) closeRetiredIfIdleLocked() {
	if !s.autoRetired || s.autoLeases > 0 || s.autoTimer != nil || s.autoRunning {
		return
	}
	s.autoClosed = true
	s.autoGeneration++
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
