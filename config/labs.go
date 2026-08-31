package config

// LabSettings persists user-level experimental feature switches. Pointer
// fields preserve layered-settings inheritance; workspaces never override
// these product capabilities.
type LabSettings struct {
	DeveloperMode *bool `toml:"developer_mode,omitempty" json:"developer_mode,omitempty"`
}

type ResolvedLabs struct {
	DeveloperMode bool `toml:"developer_mode" json:"developer_mode"`
}

func DefaultLabSettings() LabSettings {
	return LabSettings{
		DeveloperMode: boolPtr(false),
	}
}

func MergeLabSettings(parent, child LabSettings) LabSettings {
	out := parent
	if child.DeveloperMode != nil {
		out.DeveloperMode = child.DeveloperMode
	}
	return out
}

func ResolveLabs(settings LabSettings) ResolvedLabs {
	return ResolvedLabs{
		DeveloperMode: boolValue(settings.DeveloperMode, false),
	}
}
