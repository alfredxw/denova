// Package contextmaintenance owns storage-neutral context cleanup and
// compaction-health rules shared by writing sessions and game journals.
package contextmaintenance

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	CompactionHealthFailure     = "failure"
	CompactionHealthSuccess     = "success"
	CompactionHealthManualRetry = "manual_retry"
)

// ToolResultReplacement is one frozen model-context substitution.
type ToolResultReplacement struct {
	MessageIndex int64  `json:"message_index"`
	ToolCallID   string `json:"tool_call_id"`
	Placeholder  string `json:"placeholder"`
}

// ToolResultCleanup contains the storage-neutral cleanup projection.
type ToolResultCleanup struct {
	ID               string
	AgentKind        string
	SourceStart      int64
	SourceEnd        int64
	Replacements     []ToolResultReplacement
	ReclaimedTokens  int
	TriggeredAtUsage int
	EarliestChanged  int64
	WarmSuffixTokens int
	RendererVersion  string
}

// NormalizeToolResultCleanup validates and canonicalizes one cleanup
// projection. newID is called only when the caller did not supply a stable ID.
func NormalizeToolResultCleanup(value ToolResultCleanup, newID func() string) (ToolResultCleanup, error) {
	value.ID = strings.TrimSpace(value.ID)
	if value.ID == "" && newID != nil {
		value.ID = strings.TrimSpace(newID())
	}
	if value.ID == "" {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup id is required")
	}
	value.AgentKind = strings.TrimSpace(value.AgentKind)
	value.RendererVersion = strings.TrimSpace(value.RendererVersion)
	if value.SourceStart < 0 || value.SourceEnd <= value.SourceStart {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup source range [%d,%d) is invalid", value.SourceStart, value.SourceEnd)
	}
	if value.RendererVersion == "" {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup renderer version is required")
	}
	if value.ReclaimedTokens <= 0 {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup reclaimed tokens must be positive")
	}
	if value.TriggeredAtUsage < 0 || value.WarmSuffixTokens < 0 {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup usage metrics cannot be negative")
	}
	if len(value.Replacements) == 0 {
		return ToolResultCleanup{}, fmt.Errorf("tool result cleanup replacements are required")
	}

	value.Replacements = slices.Clone(value.Replacements)
	for index := range value.Replacements {
		replacement := &value.Replacements[index]
		replacement.ToolCallID = strings.TrimSpace(replacement.ToolCallID)
		if replacement.MessageIndex < value.SourceStart || replacement.MessageIndex >= value.SourceEnd {
			return ToolResultCleanup{}, fmt.Errorf("tool result cleanup replacement index %d is outside source range", replacement.MessageIndex)
		}
		if replacement.ToolCallID == "" || replacement.Placeholder == "" {
			return ToolResultCleanup{}, fmt.Errorf("tool result cleanup replacement %d requires tool_call_id and placeholder", index)
		}
	}
	sort.SliceStable(value.Replacements, func(left, right int) bool {
		if value.Replacements[left].MessageIndex == value.Replacements[right].MessageIndex {
			return value.Replacements[left].ToolCallID < value.Replacements[right].ToolCallID
		}
		return value.Replacements[left].MessageIndex < value.Replacements[right].MessageIndex
	})
	for index := 1; index < len(value.Replacements); index++ {
		if value.Replacements[index-1].MessageIndex == value.Replacements[index].MessageIndex {
			return ToolResultCleanup{}, fmt.Errorf("tool result cleanup contains a duplicate replacement target")
		}
	}
	value.EarliestChanged = value.Replacements[0].MessageIndex
	return value, nil
}

func SameToolResultCleanupIntent(existing, requested ToolResultCleanup) bool {
	return existing.ID == requested.ID && existing.AgentKind == requested.AgentKind &&
		existing.SourceStart == requested.SourceStart && existing.SourceEnd == requested.SourceEnd &&
		slices.Equal(existing.Replacements, requested.Replacements) &&
		existing.ReclaimedTokens == requested.ReclaimedTokens && existing.TriggeredAtUsage == requested.TriggeredAtUsage &&
		existing.EarliestChanged == requested.EarliestChanged && existing.WarmSuffixTokens == requested.WarmSuffixTokens &&
		existing.RendererVersion == requested.RendererVersion
}

func CloneToolResultCleanup(value ToolResultCleanup) ToolResultCleanup {
	value.Replacements = slices.Clone(value.Replacements)
	return value
}

// CompactionHealth is the storage-neutral failure-fuse state for one stable
// provider context structure.
type CompactionHealth struct {
	ID                   string
	AgentKind            string
	StructureFingerprint string
	Outcome              string
	FailureCode          string
	ConsecutiveFailures  int
}

func NormalizeCompactionHealth(value CompactionHealth) (CompactionHealth, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.AgentKind = strings.TrimSpace(value.AgentKind)
	value.StructureFingerprint = strings.TrimSpace(value.StructureFingerprint)
	value.Outcome = strings.TrimSpace(value.Outcome)
	value.FailureCode = strings.TrimSpace(value.FailureCode)
	if value.ID == "" || value.StructureFingerprint == "" {
		return CompactionHealth{}, fmt.Errorf("context compaction health requires id and structure fingerprint")
	}
	switch value.Outcome {
	case CompactionHealthFailure:
		if value.FailureCode == "" {
			return CompactionHealth{}, fmt.Errorf("failed context compaction health requires failure code")
		}
	case CompactionHealthSuccess, CompactionHealthManualRetry:
		value.FailureCode = ""
	default:
		return CompactionHealth{}, fmt.Errorf("unsupported context compaction health outcome %q", value.Outcome)
	}
	return value, nil
}

// AdvanceCompactionHealth derives the failure count from canonical prior
// state. A changed Agent or context structure starts a fresh failure series.
func AdvanceCompactionHealth(previous *CompactionHealth, next CompactionHealth) CompactionHealth {
	if next.Outcome != CompactionHealthFailure {
		next.ConsecutiveFailures = 0
		return next
	}
	next.ConsecutiveFailures = 1
	if previous != nil && strings.TrimSpace(previous.AgentKind) == strings.TrimSpace(next.AgentKind) &&
		strings.TrimSpace(previous.StructureFingerprint) == strings.TrimSpace(next.StructureFingerprint) {
		next.ConsecutiveFailures = previous.ConsecutiveFailures + 1
	}
	return next
}

func SameCompactionHealthIntent(existing, requested CompactionHealth) bool {
	return existing.ID == requested.ID && existing.AgentKind == requested.AgentKind &&
		existing.StructureFingerprint == requested.StructureFingerprint && existing.Outcome == requested.Outcome &&
		existing.FailureCode == requested.FailureCode
}
