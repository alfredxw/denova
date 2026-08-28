package harnessstate

import (
	"sort"
	"strings"
)

// AgentDebugProjection is a model-free view of the exact Draft contribution
// that would be assembled for one Agent kind. It deliberately exposes source
// identities rather than duplicating injected prompt content in another API.
type AgentDebugProjection struct {
	AgentKind        string               `json:"agent_kind"`
	PromptResource   string               `json:"prompt_resource,omitempty"`
	Contexts         []DebugContext       `json:"contexts"`
	ScriptTools      []ScriptToolMetadata `json:"script_tools"`
	SubAgents        []DebugSubAgent      `json:"subagents"`
	ToolDescriptions []string             `json:"tool_descriptions"`
}

type DebugContext struct {
	ID       string `json:"id"`
	Purpose  string `json:"purpose"`
	Resource string `json:"resource"`
}

type DebugSubAgent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Resource    string `json:"resource"`
}

func (h Harness) DebugProjection(agentKind string) AgentDebugProjection {
	agentKind = strings.TrimSpace(agentKind)
	projection := AgentDebugProjection{
		AgentKind:        agentKind,
		Contexts:         make([]DebugContext, 0),
		ScriptTools:      make([]ScriptToolMetadata, 0),
		SubAgents:        make([]DebugSubAgent, 0),
		ToolDescriptions: make([]string, 0, len(h.toolDescriptions)),
	}
	if strings.TrimSpace(h.prompts[agentKind]) != "" {
		projection.PromptResource = "prompts/" + agentKind + ".md"
	}
	for _, fragment := range h.contexts {
		if contains(fragment.Agents, agentKind) {
			projection.Contexts = append(projection.Contexts, DebugContext{
				ID: fragment.ID, Purpose: fragment.Purpose, Resource: fragment.Resource,
			})
		}
	}
	for _, tool := range h.ScriptToolMetadata() {
		if tool.Enabled && contains(tool.Agents, agentKind) {
			projection.ScriptTools = append(projection.ScriptTools, tool)
		}
	}
	for _, subAgent := range h.subAgents {
		if contains(subAgent.Parents, agentKind) {
			projection.SubAgents = append(projection.SubAgents, DebugSubAgent{
				ID: subAgent.ID, Name: subAgent.Name, Description: subAgent.Description,
				Resource: "subagents/" + subAgent.ID + ".md",
			})
		}
	}
	for name := range h.toolDescriptions {
		projection.ToolDescriptions = append(projection.ToolDescriptions, name)
	}
	sort.Strings(projection.ToolDescriptions)
	return projection
}
