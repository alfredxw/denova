package settings

import (
	"testing"

	"denova/config"
)

func TestApplyLayeredRefreshesResolvedLabSettings(t *testing.T) {
	enabled, scheduled := true, true
	interval, cap := 48, 120
	cfg := config.Config{}
	ApplyLayered(&cfg, config.LayeredSettings{Effective: config.Settings{Labs: config.LabSettings{
		ContinualLearning:              &enabled,
		ContinualLearningSchedule:      &scheduled,
		ContinualLearningIntervalHours: &interval,
		ContinualLearningTrajectoryCap: &cap,
	}}})

	if !cfg.Labs.ContinualLearning || !cfg.Labs.ContinualLearningSchedule ||
		cfg.Labs.ContinualLearningIntervalHours != interval || cfg.Labs.ContinualLearningTrajectoryCap != cap {
		t.Fatalf("runtime Labs were not refreshed: %#v", cfg.Labs)
	}
}
