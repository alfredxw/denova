package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	apptask "denova/internal/app/task"
)

const schedulerPollInterval = time.Minute

type ScheduleStatus struct {
	Enabled       bool       `json:"enabled"`
	IntervalHours int        `json:"interval_hours"`
	LastAttempt   *time.Time `json:"last_attempt,omitempty"`
	LastSuccess   *time.Time `json:"last_success,omitempty"`
	LastTaskID    string     `json:"last_task_id,omitempty"`
}

type scheduleRecord struct {
	LastAttempt time.Time `json:"last_attempt,omitempty"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastTaskID  string    `json:"last_task_id,omitempty"`
}

// StartScheduler starts the user-level periodic trigger. Manual and scheduled
// learning converge on StartTask, so live-State validation and Git history
// recording are identical.
func (service *Service) StartScheduler(ctx context.Context) {
	if service == nil || service.host == nil {
		return
	}
	service.schedulerOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		schedulerCtx, cancel := context.WithCancel(ctx)
		service.schedulerStop = cancel
		service.schedulerDone = make(chan struct{})
		go func() {
			defer close(service.schedulerDone)
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("[harness-optimizer-scheduler] panic recovered", "panic", recovered)
				}
			}()
			service.runScheduleTick(schedulerCtx, time.Now().UTC())
			ticker := time.NewTicker(schedulerPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-schedulerCtx.Done():
					return
				case now := <-ticker.C:
					service.runScheduleTick(schedulerCtx, now.UTC())
				}
			}
		}()
	})
}

func (service *Service) ScheduleStatus() (ScheduleStatus, error) {
	if service == nil || service.host == nil {
		return ScheduleStatus{}, errors.New("continual learning service is unavailable")
	}
	runtime := service.host.Runtime()
	status := ScheduleStatus{
		Enabled:       runtime.Config.Labs.ContinualLearning && runtime.Config.Labs.ContinualLearningSchedule,
		IntervalHours: runtime.Config.Labs.ContinualLearningIntervalHours,
	}
	if !runtime.Config.Labs.ContinualLearning {
		return status, nil
	}
	if err := service.initialize(); err != nil {
		return ScheduleStatus{}, err
	}
	record, err := service.readScheduleRecord()
	if err != nil {
		return ScheduleStatus{}, err
	}
	if !record.LastAttempt.IsZero() {
		lastAttempt := record.LastAttempt
		status.LastAttempt = &lastAttempt
	}
	if !record.LastSuccess.IsZero() {
		lastSuccess := record.LastSuccess
		status.LastSuccess = &lastSuccess
	}
	status.LastTaskID = record.LastTaskID
	return status, nil
}

func (service *Service) runScheduleTick(ctx context.Context, now time.Time) {
	runtime := service.host.Runtime()
	labs := runtime.Config.Labs
	if !labs.ContinualLearning || !labs.ContinualLearningSchedule {
		return
	}
	if err := service.initialize(); err != nil {
		slog.WarnContext(ctx, "[harness-optimizer-scheduler] initialize failed", "error", err)
		return
	}
	service.scheduleMu.Lock()
	defer service.scheduleMu.Unlock()
	record, err := service.readScheduleRecordUnlocked()
	if err != nil {
		slog.WarnContext(ctx, "[harness-optimizer-scheduler] read state failed", "error", err)
		return
	}
	interval := time.Duration(labs.ContinualLearningIntervalHours) * time.Hour
	if !record.LastAttempt.IsZero() && now.Sub(record.LastAttempt) < interval {
		return
	}
	if task := service.ActiveTask(); task != nil && !task.Finished() {
		return
	}
	bucket := now.Unix() / int64(interval/time.Second)
	record.LastAttempt = now
	record.LastTaskID = ""
	if err := service.writeScheduleRecordUnlocked(record); err != nil {
		slog.WarnContext(ctx, "[harness-optimizer-scheduler] persist attempt failed", "error", err)
		return
	}
	task, err := service.StartTask(ctx, Request{
		CommandID: fmt.Sprintf("harness-scheduled-%d", bucket), Trigger: TriggerScheduled,
	})
	if err != nil {
		if !errors.Is(err, ErrDisabled) {
			slog.WarnContext(ctx, "[harness-optimizer-scheduler] start failed", "error", err)
		}
		return
	}
	record.LastTaskID = task.ID()
	if err := service.writeScheduleRecordUnlocked(record); err != nil {
		slog.WarnContext(ctx, "[harness-optimizer-scheduler] persist attempt failed", "task_id", task.ID(), "error", err)
	}
	go service.observeScheduledTask(ctx, task, now)
}

func (service *Service) observeScheduledTask(ctx context.Context, task *apptask.Task, attemptedAt time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("[harness-optimizer-scheduler] observer panic recovered", "panic", recovered)
		}
	}()
	select {
	case <-ctx.Done():
		return
	case <-task.Done():
	}
	if task.Status() != apptask.Done {
		return
	}
	service.scheduleMu.Lock()
	defer service.scheduleMu.Unlock()
	record, err := service.readScheduleRecordUnlocked()
	if err != nil || record.LastTaskID != task.ID() {
		return
	}
	record.LastSuccess = attemptedAt
	if err := service.writeScheduleRecordUnlocked(record); err != nil {
		slog.WarnContext(ctx, "[harness-optimizer-scheduler] persist success failed", "task_id", task.ID(), "error", err)
	}
}

func (service *Service) readScheduleRecord() (scheduleRecord, error) {
	service.scheduleMu.Lock()
	defer service.scheduleMu.Unlock()
	return service.readScheduleRecordUnlocked()
}

func (service *Service) readScheduleRecordUnlocked() (scheduleRecord, error) {
	content, err := os.ReadFile(service.schedulePath())
	if errors.Is(err, fs.ErrNotExist) {
		return scheduleRecord{}, nil
	}
	if err != nil {
		return scheduleRecord{}, err
	}
	var record scheduleRecord
	if err := json.Unmarshal(content, &record); err != nil {
		return scheduleRecord{}, fmt.Errorf("decode Harness Optimizer schedule state: %w", err)
	}
	return record, nil
}

func (service *Service) writeScheduleRecordUnlocked(record scheduleRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := service.schedulePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".schedule-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(encoded)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryPath, path)
	}
	if err != nil {
		_ = os.Remove(temporaryPath)
	}
	return err
}

func (service *Service) schedulePath() string {
	return filepath.Join(service.dataDir, "continual-learning", "schedule.json")
}
