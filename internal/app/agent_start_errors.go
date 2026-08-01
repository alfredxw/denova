package app

import (
	"errors"

	apptask "denova/internal/app/task"
)

var (
	// ErrAgentCommandIDRequired is returned before any display task, model, or
	// canonical side effect is allocated for a root Agent request without caller identity.
	ErrAgentCommandIDRequired = errors.New("agent command_id is required")
	// ErrAgentCommandConflict means one caller identity was reused for a
	// different payload or lifecycle binding.
	ErrAgentCommandConflict = errors.New("agent command_id was already used for a different request")
	// ErrAgentReplayCapacity means every bounded display replay slot is owned by
	// live work. Admission fails before the durable Runtime command is submitted,
	// so callers may retry the same command without an uncertain acceptance.
	ErrAgentReplayCapacity = apptask.ErrReplayCapacity
	// ErrImageAgentReplayResultUnavailable means the durable image operation
	// settled but its story display projection has not become observable yet.
	ErrImageAgentReplayResultUnavailable = errors.New("replayed image Agent result is not yet projected")
	// ErrImageAgentExecution distinguishes accepted provider/tool/persistence
	// failures from request validation so HTTP clients retain uncertain IDs.
	ErrImageAgentExecution = errors.New("image Agent execution failed")
)
