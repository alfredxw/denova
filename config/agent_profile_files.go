package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// AgentProfilePath resolves a fixed Agent settings key to its existing file.
// It returns no path for unknown keys, never an inferred filesystem location.
func AgentProfilePath(kind string) (string, error) {
	if kind == "default" {
		return "main/defaults.toml", nil
	}
	for _, profile := range fixedAgentProfiles {
		if profile.Kind == kind {
			return "main/" + profile.Filename, nil
		}
	}
	return "", fmt.Errorf("unknown Agent Profile kind %q", kind)
}

// AgentProfilePatchPaths identifies fixed profiles explicitly addressed by a
// settings patch, including resets whose decoded value would otherwise look
// unchanged when the file is invalid. Collection edits are identified by the
// planner's stable-ID diff, preserving invalid unmentioned collection files.
func AgentProfilePatchPaths(changes json.RawMessage) ([]string, error) {
	if err := validateSettingsPatchObject(changes); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(changes, &fields); err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for field, value := range fields {
		switch field {
		case "agent_models", "agent_tools", "agent_prompts", "agent_skills", "agent_context", "general_sub_agents":
			var kinds map[string]json.RawMessage
			if err := json.Unmarshal(value, &kinds); err != nil {
				return nil, err
			}
			if kinds == nil {
				paths["main/defaults.toml"] = true
				for _, profile := range fixedAgentProfiles {
					paths["main/"+profile.Filename] = true
				}
			}
			for kind := range kinds {
				// These retained settings are outside the physical Profile partitions.
				if kind == "automation" || kind == "config_manager" {
					continue
				}
				path, err := AgentProfilePath(kind)
				if err != nil {
					return nil, err
				}
				paths[path] = true
			}
		case "agent_tool_parallelism", "agent_subagent_parallelism":
			paths["main/general.toml"] = true
		case "default_image_api_profile_id", "default_image_agent_id":
			paths["main/image.toml"] = true
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func encodePlainSettings(settings Settings) ([]byte, error) {
	return toml.Marshal(sanitizeEditableSettings(settings))
}

func encodeAgentProfileFiles(settings Settings) (map[string][]byte, error) {
	defaults, err := encodeAgentProfileDefaults(settings)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"main/defaults.toml": defaults}
	for _, profile := range fixedAgentProfiles {
		content, err := encodeMainAgentProfile(settings, profile)
		if err != nil {
			return nil, err
		}
		files["main/"+profile.Filename] = content
	}
	for _, agent := range SanitizeCustomAgents(settings.CustomAgents) {
		content, err := encodeCustomAgentProfile(agent)
		if err != nil {
			return nil, err
		}
		files["custom/"+agent.ID+".toml"] = content
	}
	for _, agent := range SanitizeSubAgents(settings.SubAgents) {
		content, err := encodeSubAgentProfile(agent)
		if err != nil {
			return nil, err
		}
		files["subagents/"+agent.ID+".toml"] = content
	}
	return files, nil
}

func validateAgentProfileFile(path string, content []byte) error {
	if path == "main/defaults.toml" {
		_, err := decodeAgentProfileDefaults(path, content)
		return err
	}
	for _, profile := range fixedAgentProfiles {
		if path == "main/"+profile.Filename {
			_, err := decodeMainAgentProfile(path, content, profile.Kind)
			return err
		}
	}
	switch filepath.ToSlash(filepath.Dir(path)) {
	case "custom":
		_, err := decodeCustomAgentProfile(path, content)
		return err
	case "subagents":
		_, err := decodeSubAgentProfile(path, content)
		return err
	}
	return fmt.Errorf("unknown Agent Profile path %q", strings.TrimSpace(path))
}
