package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	appsettings "denova/internal/app/settings"
)

// ErrConversationDefaultsNotSaved distinguishes a failed preference write
// from a rejected conversation update. The conversation may already be saved.
var ErrConversationDefaultsNotSaved = errors.New("conversation defaults were not saved")

// Only explicit model/runtime controls update new-conversation defaults.
// Restoring, running or creating a conversation must never remember its snapshot.
func (a *App) rememberConversationDefaults(binding ConversationConfigBinding, snapshot conversationconfig.Snapshot, patch conversationconfig.Patch) (err error) {
	defer func() {
		if err != nil {
			slog.Error("[conversation-config] remember selection failed", "agent_kind", snapshot.AgentKind, "custom_agent_id", snapshot.CustomAgentID, "error", err)
			err = fmt.Errorf("%w: %v", ErrConversationDefaultsNotSaved, err)
		}
	}()
	changes := map[string]any{}
	model := map[string]string{}
	if snapshot.AgentKind == config.AgentKindIDE || snapshot.AgentKind == config.AgentKindInteractiveStory {
		if patch.ProfileID != nil {
			model["profile_id"] = snapshot.ProfileID
		}
		if patch.ThinkingLevel != nil {
			model["thinking_level"] = snapshot.ThinkingLevel
		}
	}
	if len(model) > 0 {
		changes["agent_models"] = map[string]any{snapshot.AgentKind: model}
	}
	baseRevision := ""
	if patch.Runtime != nil {
		selection := snapshot.Engine()
		preferences := config.RuntimePreferences{Selected: selection.Kind, Codex: selection.Codex, Claude: selection.Claude, Grok: selection.Grok, Antigravity: selection.Antigravity}
		if snapshot.CustomAgentID != "" {
			// Preserve the current definition and inactive engine settings. CAS
			// protects other Agent edits because custom_agents is an array patch.
			layered, loadErr := a.SettingsService().Snapshot(appsettings.Global())
			if loadErr != nil {
				return loadErr
			}
			agents := layered.User.CustomAgents
			found := false
			for index := range agents {
				if agents[index].ID != snapshot.CustomAgentID {
					continue
				}
				if agents[index].Runtime != nil {
					retained := *agents[index].Runtime
					retained.Selected = selection.Kind
					if selection.Codex != nil {
						retained.Codex = selection.Codex
					}
					if selection.Claude != nil {
						retained.Claude = selection.Claude
					}
					if selection.Grok != nil {
						retained.Grok = selection.Grok
					}
					if selection.Antigravity != nil {
						retained.Antigravity = selection.Antigravity
					}
					preferences = retained
				}
				agents[index].Runtime = &preferences
				found = true
				break
			}
			if !found {
				return fmt.Errorf("%w: %s", config.ErrCustomAgentNotFound, snapshot.CustomAgentID)
			}
			changes["custom_agents"] = agents
			baseRevision = layered.Revisions.User
		} else {
			layered, loadErr := a.SettingsService().Snapshot(appsettings.Project(binding.ProjectID))
			if loadErr != nil {
				return loadErr
			}
			runtimeChanges := map[string]any{"agent_runtimes": map[string]any{snapshot.AgentKind: preferences}}
			if layered.Workspace.AgentRuntimes.ForAgent(snapshot.AgentKind).Selected != "" {
				// An explicit Project selector takes precedence over user defaults.
				// Update that existing scope so it cannot mask this manual choice.
				data, marshalErr := json.Marshal(runtimeChanges)
				if marshalErr != nil {
					return marshalErr
				}
				if _, err = a.SettingsService().Patch(appsettings.Project(binding.ProjectID), config.SettingsLayerWorkspace, data, ""); err != nil {
					return err
				}
			} else {
				changes["agent_runtimes"] = runtimeChanges["agent_runtimes"]
			}
		}
	}
	if len(changes) > 0 {
		data, marshalErr := json.Marshal(changes)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = a.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, data, baseRevision); err != nil {
			return err
		}
	}
	if len(changes) > 0 || patch.Runtime != nil {
		slog.Info("[conversation-config] remembered selection", "agent_kind", snapshot.AgentKind, "custom_agent_id", snapshot.CustomAgentID, "runtime", snapshot.Engine().Kind)
	}
	return nil
}
