package versions

import (
	"log"
	"time"
)

// ScheduleAutoVersion marks the workspace dirty for automatic versioning.
// Repeated changes reset the idle delay; the Git work itself always runs in
// the timer goroutine and therefore never blocks the durable file mutation.
func (s *Service) ScheduleAutoVersion(settings VersionAutoSettings) {
	settings = normalizeVersionAutoSettings(settings)

	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoClosed {
		return
	}

	s.autoGeneration++
	s.autoSettings = settings
	if !settings.TimedEnabled {
		s.stopAutoTimerLocked()
		return
	}
	if s.autoRunning {
		return
	}
	s.resetAutoTimerLocked(s.autoVersionIdleDelay, s.autoGeneration)
}

// ConfigureAutoVersion applies a settings change without marking a clean
// workspace dirty. Disabling the feature cancels any pending timer at once.
func (s *Service) ConfigureAutoVersion(settings VersionAutoSettings) {
	settings = normalizeVersionAutoSettings(settings)

	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoClosed {
		return
	}
	s.autoSettings = settings
	if !settings.TimedEnabled {
		s.autoGeneration++
		s.stopAutoTimerLocked()
		return
	}
	if s.autoTimer != nil || s.autoRunning {
		s.autoGeneration++
		if !s.autoRunning {
			s.resetAutoTimerLocked(s.autoVersionIdleDelay, s.autoGeneration)
		}
	}
}

// Close stops pending automatic version work for this workspace.
func (s *Service) Close() {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	s.autoClosed = true
	s.autoGeneration++
	s.stopAutoTimerLocked()
}

func (s *Service) resetAutoTimerLocked(delay time.Duration, generation uint64) {
	s.stopAutoTimerLocked()
	if delay <= 0 {
		delay = defaultAutoVersionIdleDelay
	}
	s.autoTimer = time.AfterFunc(delay, func() {
		s.runScheduledAutoVersion(generation)
	})
}

func (s *Service) stopAutoTimerLocked() {
	if s.autoTimer == nil {
		return
	}
	s.autoTimer.Stop()
	s.autoTimer = nil
}

func (s *Service) runScheduledAutoVersion(generation uint64) {
	started := false
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[versions] 自动版本后台任务 panic workspace=%q panic=%v", s.workspace, recovered)
			if started {
				s.finishScheduledAutoVersion(generation, 0)
			}
		}
	}()

	s.autoMu.Lock()
	if s.autoClosed || s.autoRunning || generation != s.autoGeneration {
		s.autoMu.Unlock()
		return
	}
	s.autoTimer = nil
	s.autoRunning = true
	settings := s.autoSettings
	started = true
	s.autoMu.Unlock()

	result, err := s.MaybeCreateTimed(settings)
	if err != nil {
		log.Printf("[versions] 自动版本创建失败 workspace=%q err=%v", s.workspace, err)
	} else if result.Skipped {
		log.Printf("[versions] 自动版本暂不创建 workspace=%q reason=%q", s.workspace, result.Reason)
	} else if result.Version != nil {
		log.Printf("[versions] 自动版本创建完成 workspace=%q id=%s", s.workspace, result.Version.ID)
	}
	s.finishScheduledAutoVersion(generation, result.RetryAfter)
}

func (s *Service) finishScheduledAutoVersion(generation uint64, retryAfter time.Duration) {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	s.autoRunning = false
	if s.autoClosed {
		return
	}
	if generation != s.autoGeneration {
		if s.autoSettings.TimedEnabled {
			s.resetAutoTimerLocked(s.autoVersionIdleDelay, s.autoGeneration)
		}
		return
	}
	if retryAfter > 0 && s.autoSettings.TimedEnabled {
		s.resetAutoTimerLocked(retryAfter, generation)
	}
}
