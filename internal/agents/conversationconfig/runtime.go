package conversationconfig

import (
	"errors"

	"denova/config"
)

var ErrRuntimeCapabilityUnsupported = errors.New("runtime does not support the requested configuration")

// Engine resolves only the persisted selection. Missing selections in released
// journals mean Native even when the Agent's current default is external.
func (selection Config) Engine() config.RuntimeSelection {
	if selection.Runtime == nil {
		return config.RuntimeSelection{Kind: config.RuntimeNative}
	}
	return cloneSelection(*selection.Runtime)
}

func cloneSelection(selection config.RuntimeSelection) config.RuntimeSelection {
	if selection.Codex != nil {
		value := *selection.Codex
		selection.Codex = &value
	}
	if selection.Claude != nil {
		value := *selection.Claude
		selection.Claude = &value
	}
	return selection
}

// Clone detaches mutable engine/Agent definitions from the persisted snapshot.
// Returning a snapshot must not let a draft mutate the canonical in-memory view.
func (selection Config) Clone() Config {
	if selection.Runtime != nil {
		value := cloneSelection(*selection.Runtime)
		selection.Runtime = &value
	}
	if selection.CustomAgent != nil {
		selection.CustomAgent = cloneCustomAgent(*selection.CustomAgent)
	}
	return selection
}

// LegacyDefault deliberately ignores newly configured engine defaults when
// initializing an existing conversation that predates selection snapshots.
func LegacyDefault(runtime *config.Config, agentKind string) Config {
	clone := cloneRuntimeConfig(runtime)
	clone.AgentRuntimes = config.AgentRuntimeSettings{}
	return Default(&clone, agentKind)
}
