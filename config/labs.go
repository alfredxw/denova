package config

// LabSettings persists user-level experimental feature switches. Pointer
// fields preserve layered-settings inheritance; workspaces never override
// these product capabilities.
type LabSettings struct {
	DeveloperMode                  *bool `toml:"developer_mode,omitempty" json:"developer_mode,omitempty"`
	ContinualLearning              *bool `toml:"continual_learning,omitempty" json:"continual_learning,omitempty"`
	ContinualLearningSchedule      *bool `toml:"continual_learning_schedule,omitempty" json:"continual_learning_schedule,omitempty"`
	ContinualLearningIntervalHours *int  `toml:"continual_learning_interval_hours,omitempty" json:"continual_learning_interval_hours,omitempty"`
	ContinualLearningTrajectoryCap *int  `toml:"continual_learning_trajectory_cap,omitempty" json:"continual_learning_trajectory_cap,omitempty"`
}

type ResolvedLabs struct {
	DeveloperMode                  bool `toml:"developer_mode" json:"developer_mode"`
	ContinualLearning              bool `toml:"continual_learning" json:"continual_learning"`
	ContinualLearningSchedule      bool `toml:"continual_learning_schedule" json:"continual_learning_schedule"`
	ContinualLearningIntervalHours int  `toml:"continual_learning_interval_hours" json:"continual_learning_interval_hours"`
	ContinualLearningTrajectoryCap int  `toml:"continual_learning_trajectory_cap" json:"continual_learning_trajectory_cap"`
}

const (
	DefaultContinualLearningIntervalHours = 24
	DefaultContinualLearningTrajectoryCap = 50
	MaxContinualLearningIntervalHours     = 24 * 30
	MaxContinualLearningTrajectoryCap     = 500
)

func DefaultLabSettings() LabSettings {
	return LabSettings{
		DeveloperMode:                  boolPtr(false),
		ContinualLearning:              boolPtr(false),
		ContinualLearningSchedule:      boolPtr(false),
		ContinualLearningIntervalHours: intPtr(DefaultContinualLearningIntervalHours),
		ContinualLearningTrajectoryCap: intPtr(DefaultContinualLearningTrajectoryCap),
	}
}

func MergeLabSettings(parent, child LabSettings) LabSettings {
	out := parent
	if child.DeveloperMode != nil {
		out.DeveloperMode = child.DeveloperMode
	}
	if child.ContinualLearning != nil {
		out.ContinualLearning = child.ContinualLearning
	}
	if child.ContinualLearningSchedule != nil {
		out.ContinualLearningSchedule = child.ContinualLearningSchedule
	}
	if child.ContinualLearningIntervalHours != nil {
		out.ContinualLearningIntervalHours = child.ContinualLearningIntervalHours
	}
	if child.ContinualLearningTrajectoryCap != nil {
		out.ContinualLearningTrajectoryCap = child.ContinualLearningTrajectoryCap
	}
	return out
}

func ResolveLabs(settings LabSettings) ResolvedLabs {
	return ResolvedLabs{
		DeveloperMode:                  boolValue(settings.DeveloperMode, false),
		ContinualLearning:              boolValue(settings.ContinualLearning, false),
		ContinualLearningSchedule:      boolValue(settings.ContinualLearningSchedule, false),
		ContinualLearningIntervalHours: boundedLabInt(settings.ContinualLearningIntervalHours, DefaultContinualLearningIntervalHours, 1, MaxContinualLearningIntervalHours),
		ContinualLearningTrajectoryCap: boundedLabInt(settings.ContinualLearningTrajectoryCap, DefaultContinualLearningTrajectoryCap, 1, MaxContinualLearningTrajectoryCap),
	}
}

func boundedLabInt(value *int, fallback, minimum, maximum int) int {
	if value == nil || *value < minimum || *value > maximum {
		return fallback
	}
	return *value
}
