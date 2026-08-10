// Package compaction owns application-level frozen compaction mutations and
// cold restore plans shared by writing and game conversations.
package compactionapp

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func SessionRecord(id, agentKind string, sourceStart, sourceEnd int, result agentcompaction.Result) session.ContextCompaction {
	if strings.TrimSpace(result.TriggerReason) == "" {
		result.TriggerReason = "manual"
	}
	return session.ContextCompaction{
		ID: id, CompactionCheckpoint: agentcompaction.NewCheckpoint(agentKind, result),
		SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd, SourceMessageCount: result.SourceMessageCount,
	}
}

func StoryEvent(id, expectedParent string, sourceTurns int, result agentcompaction.Result) interactive.ContextCompactionEvent {
	if strings.TrimSpace(result.TriggerReason) == "" {
		result.TriggerReason = "manual"
	}
	return interactive.ContextCompactionEvent{
		ID: id, CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindInteractiveStory, result),
		SourceTurnCount: sourceTurns, ExpectedParentID: &expectedParent,
	}
}

func ResultFromSession(record session.ContextCompaction) agentcompaction.Result {
	result := agentcompaction.ResultFromCheckpoint(record.CompactionCheckpoint)
	result.SourceMessageCount = record.SourceMessageCount
	return result
}

func ResultFromStory(event interactive.ContextCompactionEvent) agentcompaction.Result {
	return agentcompaction.ResultFromCheckpoint(event.CompactionCheckpoint)
}

func ResolveCommandID(requested, fallback string) (string, error) {
	commandID := strings.TrimSpace(requested)
	if commandID == "" {
		commandID = strings.TrimSpace(fallback)
	}
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return "", err
	}
	return commandID, nil
}

func WritingBinding(workspace, sessionID string) agentrun.RuntimeBinding {
	return agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID}
}

func StoryBinding(workspace, storyID, branchID string) agentrun.RuntimeBinding {
	return agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: storyID, BranchID: branchID}
}

func RecoveryActionFor(action agentstructural.Action) agentexecution.RuntimeRecoveryActionKind {
	switch action {
	case agentstructural.Compact:
		return agentexecution.RuntimeRecoveryCompactContext
	case agentstructural.Remove:
		return agentexecution.RuntimeRecoveryRemoveCompaction
	default:
		return ""
	}
}

func SameSessionMutation(actual, expected session.ContextCompaction) bool {
	actual.Type, expected.Type = "", ""
	actual.CreatedAt, expected.CreatedAt = time.Time{}, time.Time{}
	actual.ContextRevision, expected.ContextRevision = 0, 0
	// Cursor fields are a derived migration of legacy index bounds. A frozen
	// pre-commit mutation may contain only indices while the canonical record
	// read through the projection has stable cursors filled in.
	if expected.SourceStartCursor == 0 {
		actual.SourceStartCursor = 0
	}
	if expected.SourceEndCursor == 0 {
		actual.SourceEndCursor = 0
	}
	return reflect.DeepEqual(actual, expected)
}

func SameSessionRemoval(actual, expected session.ContextCompactionRemoval) bool {
	actual.Type, expected.Type = "", ""
	actual.CreatedAt, expected.CreatedAt = time.Time{}, time.Time{}
	actual.ContextRevision, expected.ContextRevision = 0, 0
	if expected.SourceStartCursor == 0 {
		actual.SourceStartCursor = 0
	}
	if expected.SourceEndCursor == 0 {
		actual.SourceEndCursor = 0
	}
	return reflect.DeepEqual(actual, expected)
}

func SessionRevision(revision uint64) string {
	return fmt.Sprintf("session-context:%d", revision)
}
