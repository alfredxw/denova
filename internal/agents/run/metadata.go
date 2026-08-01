package agentrun

// TraceMetadata is the bounded product identity attached to one run. A
// conversation may fill fields such as TurnID only after canonical commit, so
// callers resolve it again when the run finishes.
type TraceMetadata struct {
	StoryID         string `json:"story_id,omitempty"`
	BranchID        string `json:"branch_id,omitempty"`
	TurnID          string `json:"turn_id,omitempty"`
	MaintenanceTask string `json:"maintenance_task,omitempty"`
}

// TraceMetadataReporter exposes canonical identity discovered during a run.
type TraceMetadataReporter interface {
	RunTraceMetadata() TraceMetadata
}
