package agents

import (
	"fmt"
	"sort"
	"strings"
)

// PersistedToolResultReplacement is the storage-neutral shape shared by
// Writing and Game cleanup journals. Storage adapters translate only their
// local DTOs; merge, ordering, and reclaimed-token semantics stay identical.
type PersistedToolResultReplacement struct {
	MessageIndex int64
	ToolCallID   string
	Placeholder  string
}

type MergedToolResultCleanup struct {
	SourceStart     int64
	SourceEnd       int64
	Replacements    []PersistedToolResultReplacement
	ReclaimedTokens int
}

// MergeToolResultCleanup combines an existing append-only projection with
// newly resolved replacements. resolved indexes are relative to the canonical
// slice and indexOffset translates them into the storage domain's absolute
// coordinate space.
func MergeToolResultCleanup(
	existing []PersistedToolResultReplacement,
	resolved []ToolResultCleanupReplacement,
	indexOffset int,
	sourceEnd int64,
	previousReclaimed, plannedReclaimed int,
) (MergedToolResultCleanup, error) {
	if indexOffset < 0 || sourceEnd < int64(indexOffset) {
		return MergedToolResultCleanup{}, fmt.Errorf("cleanup canonical bounds are invalid")
	}
	merged := make(map[int64]PersistedToolResultReplacement, len(existing)+len(resolved))
	sourceStart := sourceEnd
	for _, replacement := range existing {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= sourceEnd || strings.TrimSpace(replacement.ToolCallID) == "" {
			return MergedToolResultCleanup{}, fmt.Errorf("existing cleanup replacement is outside canonical bounds")
		}
		merged[replacement.MessageIndex] = replacement
		if replacement.MessageIndex < sourceStart {
			sourceStart = replacement.MessageIndex
		}
	}
	reclaimed := max(0, previousReclaimed)
	for _, replacement := range resolved {
		index := int64(indexOffset + replacement.MessageIndex)
		if replacement.MessageIndex < 0 || index < 0 || index >= sourceEnd || strings.TrimSpace(replacement.ToolCallID) == "" {
			return MergedToolResultCleanup{}, fmt.Errorf("resolved cleanup replacement is outside canonical bounds")
		}
		_, existed := merged[index]
		merged[index] = PersistedToolResultReplacement{
			MessageIndex: index, ToolCallID: strings.TrimSpace(replacement.ToolCallID), Placeholder: replacement.Placeholder,
		}
		if !existed {
			reclaimed += max(0, replacement.OriginalTokens-replacement.PlaceholderTokens)
		}
		if index < sourceStart {
			sourceStart = index
		}
	}
	if sourceStart >= sourceEnd {
		return MergedToolResultCleanup{}, fmt.Errorf("cleanup has no canonical replacement targets")
	}
	ordered := make([]PersistedToolResultReplacement, 0, len(merged))
	for _, replacement := range merged {
		ordered = append(ordered, replacement)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].MessageIndex < ordered[j].MessageIndex })
	return MergedToolResultCleanup{
		SourceStart: sourceStart, SourceEnd: sourceEnd, Replacements: ordered,
		ReclaimedTokens: max(reclaimed, plannedReclaimed),
	}, nil
}
