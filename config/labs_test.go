package config

import "testing"

func TestResolveLabsDefaultsAndBounds(t *testing.T) {
	resolved := ResolveLabs(LabSettings{})
	if resolved.DeveloperMode || resolved.ContinualLearningSchedule || resolved.ContinualLearningIntervalHours != 24 || resolved.ContinualLearningTrajectoryCap != 100 {
		t.Fatalf("unexpected Lab defaults %#v", resolved)
	}

	enabled, scheduled := true, true
	tooLargeInterval, tooSmallCap := 10_000, 0
	resolved = ResolveLabs(LabSettings{
		DeveloperMode:                  &enabled,
		ContinualLearningSchedule:      &scheduled,
		ContinualLearningIntervalHours: &tooLargeInterval,
		ContinualLearningTrajectoryCap: &tooSmallCap,
	})
	if !resolved.DeveloperMode || !resolved.ContinualLearningSchedule {
		t.Fatalf("Lab flags were not resolved: %#v", resolved)
	}
	if resolved.ContinualLearningIntervalHours != DefaultContinualLearningIntervalHours || resolved.ContinualLearningTrajectoryCap != DefaultContinualLearningTrajectoryCap {
		t.Fatalf("Lab bounds were not enforced: %#v", resolved)
	}
}

func TestMergeLabsUsesChildFieldsIndependently(t *testing.T) {
	enabled, disabled := true, false
	interval, cap := 48, 120
	merged := MergeLabSettings(
		LabSettings{DeveloperMode: &enabled, ContinualLearningIntervalHours: &interval},
		LabSettings{ContinualLearningSchedule: &disabled, ContinualLearningTrajectoryCap: &cap},
	)
	if merged.DeveloperMode != &enabled || merged.ContinualLearningSchedule != &disabled ||
		merged.ContinualLearningIntervalHours != &interval || merged.ContinualLearningTrajectoryCap != &cap {
		t.Fatalf("unexpected merged Labs %#v", merged)
	}
}
