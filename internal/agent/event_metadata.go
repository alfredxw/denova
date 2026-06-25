package agent

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
)

type agentEventMetadata struct {
	AgentName     string
	RootAgentName string
	RunPath       []string
	SubAgent      bool
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
	return meta
}

func (m agentEventMetadata) appendTo(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	if m.AgentName != "" {
		data["agent_name"] = m.AgentName
	}
	if m.RootAgentName != "" {
		data["root_agent_name"] = m.RootAgentName
	}
	if len(m.RunPath) > 0 {
		data["run_path"] = append([]string(nil), m.RunPath...)
	}
	data["subagent"] = m.SubAgent
	return data
}

func eventMetadataFromData(data interface{}) agentEventMetadata {
	meta := agentEventMetadata{}
	switch typed := data.(type) {
	case map[string]string:
		meta.AgentName = typed["agent_name"]
		meta.RootAgentName = typed["root_agent_name"]
		meta.SubAgent = strings.EqualFold(typed["subagent"], "true")
	case map[string]interface{}:
		meta.AgentName = eventDataString(typed, "agent_name")
		meta.RootAgentName = eventDataString(typed, "root_agent_name")
		meta.SubAgent = eventDataBool(typed, "subagent")
		if raw, ok := typed["run_path"]; ok {
			meta.RunPath = stringSliceFromAny(raw)
		}
	}
	return meta
}

func (m agentEventMetadata) sameSource(other agentEventMetadata) bool {
	return m.AgentName == other.AgentName &&
		m.RootAgentName == other.RootAgentName &&
		m.SubAgent == other.SubAgent &&
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
