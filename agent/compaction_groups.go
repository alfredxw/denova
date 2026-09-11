package agent

// interactionBoundaries returns complete assistant steps, keeping the whole
// call batch with all its results. A user instruction belongs to its first
// assistant step. An incomplete or malformed suffix adds no completed boundary,
// so it cannot displace the newest completed tool group from the retained tail.
func interactionBoundaries(messages []*Message) []int {
	boundaries := []int{0}
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message == nil || message.Role != Assistant {
			continue
		}
		pending := make(map[string]bool, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			if call.ID == "" || pending[call.ID] {
				return boundaries
			}
			pending[call.ID] = true
		}
		end := index + 1
		for end < len(messages) && messages[end] != nil && messages[end].Role == ToolRole {
			id := messages[end].ToolCallID
			if !pending[id] {
				return boundaries
			}
			delete(pending, id)
			end++
		}
		if len(pending) != 0 {
			return boundaries
		}
		boundaries = append(boundaries, end)
		index = end - 1
	}
	return boundaries
}

// compactionGroups maps offered groups back to private journal coordinates.
func compactionGroups(raw, projected []*Message, current compactionRecord, present bool) ([]CompactionGroup, []int, int) {
	boundaries := interactionBoundaries(raw)
	start := 0
	if present && !current.Removed {
		start = current.ReplacementTo
	}
	groups := make([]CompactionGroup, 0)
	ends := make([]int, 0)
	for _, end := range boundaries[1:max(1, len(boundaries)-1)] {
		if end <= start {
			continue
		}
		groups = append(groups, CompactionGroup{Messages: cloneMessages(projected[start:end])})
		ends = append(ends, end)
		start = end
	}
	return groups, ends, compactionMessagesBytes(projected[start:])
}
