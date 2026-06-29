package agent

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
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
}

func metadataForAgentEvent(event *adk.AgentEvent, rootAgentName string) agentEventMetadata {
	meta := agentEventMetadata{
		RootAgentName: strings.TrimSpace(rootAgentName),
	}
	if event == nil {
		return meta
	}
	meta.AgentName = strings.TrimSpace(event.AgentName)
	if len(event.RunPath) > 0 {
		meta.RunPath = make([]string, 0, len(event.RunPath))
		for _, step := range event.RunPath {
			name := strings.TrimSpace(step.String())
			if name == "" {
				continue
			}
			meta.RunPath = append(meta.RunPath, name)
		}
	}
	if meta.AgentName == "" && len(meta.RunPath) > 0 {
		meta.AgentName = meta.RunPath[len(meta.RunPath)-1]
	}
	if meta.RootAgentName == "" {
		if len(meta.RunPath) > 0 {
			meta.RootAgentName = meta.RunPath[0]
		} else {
			meta.RootAgentName = meta.AgentName
		}
	}
	meta.SubAgent = meta.AgentName != "" && meta.RootAgentName != "" && meta.AgentName != meta.RootAgentName
	if meta.SubAgent {
		meta.SubAgentType = meta.AgentName
	}
	return meta
}

type subAgentSessionTracker struct {
	runID        string
	counter      int
	activeSource string
	activeID     string
}

func newSubAgentSessionTracker(runID string) *subAgentSessionTracker {
	return &subAgentSessionTracker{runID: strings.TrimSpace(runID)}
}

func (t *subAgentSessionTracker) decorate(meta agentEventMetadata) agentEventMetadata {
	if t == nil {
		return meta
	}
	meta.RunID = t.runID
	if !meta.SubAgent {
		t.activeSource = ""
		t.activeID = ""
		return meta
	}
	if meta.SubAgentType == "" {
		meta.SubAgentType = meta.AgentName
	}
	source := subAgentSourceKey(meta)
	if source == "" {
		source = meta.AgentName
	}
	if source != t.activeSource || t.activeID == "" {
		t.counter++
		t.activeSource = source
		t.activeID = buildSubAgentSessionID(t.runID, meta.AgentName, t.counter)
	}
	meta.SubAgentSessionID = t.activeID
	return meta
}

func subAgentSourceKey(meta agentEventMetadata) string {
	parts := []string{meta.RootAgentName, meta.AgentName}
	parts = append(parts, meta.RunPath...)
	return strings.Join(parts, "\x00")
}

func buildSubAgentSessionID(runID, agentName string, index int) string {
	runID = sanitizeSubAgentSessionPart(runID)
	if runID == "" {
		runID = "run"
	}
	agentName = sanitizeSubAgentSessionPart(agentName)
	if agentName == "" {
		agentName = "subagent"
	}
	return fmt.Sprintf("%s-subagent-%02d-%s", runID, index, agentName)
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
		meta.SubAgent = strings.EqualFold(typed["subagent"], "true")
	case map[string]interface{}:
		meta.AgentKind = eventDataString(typed, "agent_kind")
		meta.RunID = eventDataString(typed, "run_id")
		meta.AgentName = eventDataString(typed, "agent_name")
		meta.RootAgentName = eventDataString(typed, "root_agent_name")
		meta.SubAgentSessionID = eventDataString(typed, "subagent_session_id")
		meta.SubAgentType = eventDataString(typed, "subagent_type")
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
