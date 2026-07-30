package session

import "strings"

// LatestModelPromptUsage returns the final provider request usage from the
// newest completed run for one Agent. Usage is display-only telemetry, but it
// survives journal reload and therefore calibrates the first pressure decision
// of the next user turn without injecting telemetry into model messages.
func (s *Session) LatestModelPromptUsage(agentKind string) (promptTokens, cachedTokens int, ok bool) {
	if s == nil {
		return 0, 0, false
	}
	agentKind = strings.TrimSpace(agentKind)
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := len(s.records) - 1; index >= 0; index-- {
		record := s.records[index]
		if modelPromptUsageInvalidatedBy(record, agentKind) {
			// A cleanup/checkpoint changed the provider-visible projection after
			// this usage sample. Until a request against that new projection
			// returns usage, pressure must rely on the rebuilt local estimate.
			return 0, 0, false
		}
		if record.kind != historyTypeDisplay || record.display == nil || record.display.Role != "token_usage" {
			continue
		}
		usage := record.display
		if agentKind != "" && strings.TrimSpace(usage.AgentKind) != agentKind {
			continue
		}
		for callIndex := len(usage.UsageCalls) - 1; callIndex >= 0; callIndex-- {
			call := usage.UsageCalls[callIndex]
			if call.PromptTokens > 0 {
				return call.PromptTokens, min(call.PromptTokens, max(0, call.CachedPromptTokens)), true
			}
		}
		// Older journals may only contain aggregate usage. It is comparable to
		// one request only when the run made at most one model call.
		if usage.PromptTokens > 0 && usage.ModelCalls <= 1 {
			return usage.PromptTokens, min(usage.PromptTokens, max(0, usage.CachedPromptTokens)), true
		}
		return 0, 0, false
	}
	return 0, 0, false
}

func modelPromptUsageInvalidatedBy(record historyRecord, agentKind string) bool {
	switch record.kind {
	case historyTypeClear:
		return true
	case historyTypeCompaction:
		return record.compaction != nil && contextRecordMatchesAgent(record.compaction.AgentKind, agentKind)
	case historyTypeCompactionRemoved:
		return record.compactionRemoval != nil && contextRecordMatchesAgent(record.compactionRemoval.AgentKind, agentKind)
	case historyTypeToolResultCleanup:
		return record.toolResultCleanup != nil && contextRecordMatchesAgent(record.toolResultCleanup.AgentKind, agentKind)
	default:
		return false
	}
}
