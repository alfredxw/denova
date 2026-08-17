package chat

import (
	"denova/internal/agents/run"
	"fmt"
	"strings"

	agentplan "denova/internal/agents/plan"
)

type agentEventMetadata struct {
	AgentKind         string
	RunID             string
	AgentName         string
	RootAgentName     string
	RunPath           []string
	SubAgent          bool
	SubAgentSessionID string
	SubAgentType      string
	ParentCallID      string
}

func (m agentEventMetadata) planMetadata() agentplan.Metadata {
	return agentplan.Metadata{
		AgentKind: m.AgentKind, RunID: m.RunID, AgentName: m.AgentName,
		RootAgentName: m.RootAgentName, RunPath: append([]string(nil), m.RunPath...),
		SubAgent: m.SubAgent, SubAgentSessionID: m.SubAgentSessionID, SubAgentType: m.SubAgentType,
	}
}

func planEventEmitter(emit func(agentrun.Event)) func(agentplan.Event) {
	if emit == nil {
		return nil
	}
	return func(event agentplan.Event) {
		emit(agentrun.Event{Type: event.Type, Data: event.Data})
	}
}

func sanitizeSubAgentSessionPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-' || r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}
	return strings.Trim(sb.String(), "-_")
}

func (m agentEventMetadata) appendTo(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	if m.AgentName != "" {
		data["agent_name"] = m.AgentName
	}
	if m.AgentKind != "" {
		data["agent_kind"] = m.AgentKind
	}
	if m.RunID != "" {
		data["run_id"] = m.RunID
	}
	if m.RootAgentName != "" {
		data["root_agent_name"] = m.RootAgentName
	}
	if len(m.RunPath) > 0 {
		data["run_path"] = append([]string(nil), m.RunPath...)
	}
	if m.SubAgentSessionID != "" {
		data["subagent_session_id"] = m.SubAgentSessionID
	}
	if m.SubAgentType != "" {
		data["subagent_type"] = m.SubAgentType
	}
	if m.ParentCallID != "" {
		data["parent_call_id"] = m.ParentCallID
	}
	data["subagent"] = m.SubAgent
	return data
}

func eventMetadataFromData(data interface{}) agentEventMetadata {
	meta := agentEventMetadata{}
	switch typed := data.(type) {
	case map[string]string:
		meta.AgentKind = typed["agent_kind"]
		meta.RunID = typed["run_id"]
		meta.AgentName = typed["agent_name"]
		meta.RootAgentName = typed["root_agent_name"]
		meta.SubAgentSessionID = typed["subagent_session_id"]
		meta.SubAgentType = typed["subagent_type"]
		meta.ParentCallID = typed["parent_call_id"]
		meta.SubAgent = strings.EqualFold(typed["subagent"], "true")
	case map[string]interface{}:
		meta.AgentKind = eventDataString(typed, "agent_kind")
		meta.RunID = eventDataString(typed, "run_id")
		meta.AgentName = eventDataString(typed, "agent_name")
		meta.RootAgentName = eventDataString(typed, "root_agent_name")
		meta.SubAgentSessionID = eventDataString(typed, "subagent_session_id")
		meta.SubAgentType = eventDataString(typed, "subagent_type")
		meta.ParentCallID = eventDataString(typed, "parent_call_id")
		meta.SubAgent = eventDataBool(typed, "subagent")
		if raw, ok := typed["run_path"]; ok {
			meta.RunPath = stringSliceFromAny(raw)
		}
	}
	if meta.SubAgent && meta.SubAgentType == "" {
		meta.SubAgentType = meta.AgentName
	}
	return meta
}

func (m agentEventMetadata) sameSource(other agentEventMetadata) bool {
	return m.RunID == other.RunID &&
		m.AgentName == other.AgentName &&
		m.RootAgentName == other.RootAgentName &&
		m.SubAgent == other.SubAgent &&
		m.SubAgentSessionID == other.SubAgentSessionID &&
		m.ParentCallID == other.ParentCallID &&
		strings.Join(m.RunPath, "\x00") == strings.Join(other.RunPath, "\x00")
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(eventAnyString(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func eventAnyString(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
