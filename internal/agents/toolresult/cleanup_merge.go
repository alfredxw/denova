package toolresult

import (
	"fmt"
	"sort"
	"strings"
)

// PersistedReplacement is the storage-neutral shape shared by
// Writing and Game cleanup journals. Storage adapters translate only their
// local DTOs; merge, ordering, and reclaimed-token semantics stay identical.
type PersistedReplacement struct {
	MessageIndex int64
	ToolCallID   string
	Placeholder  string
}

type MergedCleanup struct {
	SourceStart     int64
	SourceEnd       int64
	Replacements    []PersistedReplacement
	ReclaimedTokens int
}

// MergeCleanup combines an existing append-only projection with
// newly resolved replacements. resolved indexes are relative to the canonical
// slice and indexOffset translates them into the storage domain's absolute
// coordinate space.
func MergeCleanup(
	existing []PersistedReplacement,
	resolved []CleanupReplacement,
	indexOffset int,
	sourceEnd int64,
	previousReclaimed, plannedReclaimed int,
) (MergedCleanup, error) {
	if indexOffset < 0 || sourceEnd < int64(indexOffset) {
		return MergedCleanup{}, fmt.Errorf("cleanup canonical bounds are invalid")
	}
	merged := make(map[int64]PersistedReplacement, len(existing)+len(resolved))
	sourceStart := sourceEnd
	for _, replacement := range existing {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= sourceEnd || strings.TrimSpace(replacement.ToolCallID) == "" {
			return MergedCleanup{}, fmt.Errorf("existing cleanup replacement is outside canonical bounds")
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
			return MergedCleanup{}, fmt.Errorf("resolved cleanup replacement is outside canonical bounds")
		}
		_, existed := merged[index]
		merged[index] = PersistedReplacement{
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
		return MergedCleanup{}, fmt.Errorf("cleanup has no canonical replacement targets")
	}
	ordered := make([]PersistedReplacement, 0, len(merged))
	for _, replacement := range merged {
		ordered = append(ordered, replacement)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].MessageIndex < ordered[j].MessageIndex })
	return MergedCleanup{
		SourceStart: sourceStart, SourceEnd: sourceEnd, Replacements: ordered,
		ReclaimedTokens: max(reclaimed, plannedReclaimed),
	}, nil
}
