package config

import "strings"

const (
	AgentContractGeneralProject = "project.general.v1"
	AgentContractWritingPrimary = "writing.primary.v1"
	AgentContractGameNarrator   = "game.narrator.v1"
	AgentContractImageCreator   = "image.creator.v1"
)

// AgentContractDefinition is the stable runtime boundary that a user-owned
// Agent plugs into. Contracts own protocol, permissions, and capability
// ceilings; the Agent definition owns behavior inside that boundary.
type AgentContractDefinition struct {
	ID             string `json:"id"`
	RuntimeKind    string `json:"runtime_kind"`
	TitleKey       string `json:"title_key"`
	DescriptionKey string `json:"description_key"`
}

var agentContractRegistry = []AgentContractDefinition{
	{ID: AgentContractGeneralProject, RuntimeKind: AgentKindGeneral, TitleKey: "agents.general.title", DescriptionKey: "agents.contract.general"},
	{ID: AgentContractWritingPrimary, RuntimeKind: AgentKindIDE, TitleKey: "agents.ide.title", DescriptionKey: "agents.contract.writing"},
	{ID: AgentContractGameNarrator, RuntimeKind: AgentKindInteractiveStory, TitleKey: "agents.interactiveStory.title", DescriptionKey: "agents.contract.game"},
	{ID: AgentContractImageCreator, RuntimeKind: AgentKindImage, TitleKey: "agents.image.title", DescriptionKey: "agents.contract.image"},
}

func AgentContractDefinitions() []AgentContractDefinition {
	return append([]AgentContractDefinition(nil), agentContractRegistry...)
}

func LookupAgentContract(id string) (AgentContractDefinition, bool) {
	id = strings.TrimSpace(id)
	for _, definition := range agentContractRegistry {
		if definition.ID == id {
			return definition, true
		}
	}
	return AgentContractDefinition{}, false
}

func AgentContractForRuntimeKind(kind string) (AgentContractDefinition, bool) {
	kind = strings.TrimSpace(kind)
	for _, definition := range agentContractRegistry {
		if definition.RuntimeKind == kind {
			return definition, true
		}
	}
	return AgentContractDefinition{}, false
}

// IsReservedAgentID keeps user-owned Agent identities distinct from every
// current or retained built-in runtime selector.
func IsReservedAgentID(id string) bool {
	switch strings.TrimSpace(id) {
	case AgentKindIDE, AgentKindGeneral, AgentKindInteractiveStory,
		AgentKindConfigManager, AgentKindVersionSummary, AgentKindToolAgent, AgentKindImage, AgentKindAutomation:
		return true
	default:
		return false
	}
}
