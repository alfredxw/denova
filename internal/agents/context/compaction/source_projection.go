package compaction

import (
	"errors"

	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

// projectCompactionSource carries the lifecycle context projection through domain adapters
// that rebuild their source from canonical history. Only exact substitutions
// supplied by Agent are accepted; the final fork still validates every source
// message against the unchanged provider snapshot.
func projectCompactionSource(request agent.CompactionCompactRequest, policy toolresult.ContextPolicy) ([]*agent.Message, error) {
	if request.ContextMessages == nil {
		return request.SourceMessages, nil
	}
	original := toolresult.ApplyContextPolicy(request.Messages, policy)
	projected := toolresult.ApplyContextPolicy(request.ContextMessages, policy)
	if len(original) != len(projected) {
		return nil, errors.New("Compaction context projection changed history positions")
	}
	type substitution struct{ original, projected *agent.Message }
	replacements := make(map[string][]substitution)
	for index, before := range original {
		after := projected[index]
		if !sameProviderVisibleMessage(before, after) {
			if before == nil || after == nil || before.Role != agent.ToolRole || after.Role != agent.ToolRole || before.ToolCallID != after.ToolCallID {
				return nil, errors.New("Compaction context projection changed a non-tool message")
			}
			replacements[before.ToolCallID] = append(replacements[before.ToolCallID], substitution{before, after})
		}
	}
	if len(replacements) == 0 {
		return request.SourceMessages, nil
	}
	if request.ModelSnapshot == nil {
		return nil, errors.New("Compaction context projection requires the final primary snapshot")
	}
	primary := request.ModelSnapshot.Messages()
	positions, _, matched := locateCompactionSource(primary, request.SourceMessages, func(actual, source *agent.Message) bool {
		if sameProviderVisibleMessage(actual, source) {
			return true
		}
		if source == nil || source.Role != agent.ToolRole {
			return false
		}
		for _, replacement := range replacements[source.ToolCallID] {
			if sameProviderVisibleMessage(source, replacement.original) && sameProviderVisibleMessage(actual, replacement.projected) {
				return true
			}
		}
		return false
	})
	if !matched {
		return nil, errors.New("canonical compaction source does not match the final primary context projection")
	}
	result := make([]*agent.Message, 0, len(positions))
	for _, message := range request.SourceMessages {
		if message == nil {
			continue
		}
		_, locator := compactionSourceMatchMessage(message)
		visible := primary[positions[len(result)]].Clone()
		visible.ReasoningContent = ""
		if locator != "" {
			visible.Content = locator + "\n" + visible.Content
		}
		result = append(result, visible)
	}
	return result, nil
}
