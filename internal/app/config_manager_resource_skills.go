package app

import (
	"context"
	"log"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agent"
	novaskills "denova/internal/skills"
)

const (
	configManagerResourceSkillMaxBytes      = 12 * 1024
	configManagerResourceSkillMaxTotalBytes = 32 * 1024

	configManagerAutomationSkill    = "automation-config"
	configManagerStoryMemorySkill   = "story-memory-config"
	configManagerTellerSkill        = "teller-config"
	configManagerStoryDirectorSkill = "story-director-config"
	configManagerImagePresetSkill   = "image-preset-config"
	configManagerSkillsSkill        = "skills-creator"
	configManagerAgentConfigSkill   = "agent-config"
	configManagerLoreSkill          = "lore"
)

func loadConfigManagerResourceSkills(ctx context.Context, cfg *config.Config, req ConfigManagerRequest) []agent.ConfigManagerResourceSkill {
	names := configManagerResourceSkillNames(req)
	if len(names) == 0 || cfg == nil {
		return nil
	}
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.NovaDir, cfg.Workspace),
		config.AgentKindConfigManager,
		config.ResolveAgentSkillOverrides(cfg, config.AgentKindConfigManager),
	)
	loaded := make([]agent.ConfigManagerResourceSkill, 0, len(names))
	remaining := configManagerResourceSkillMaxTotalBytes
	for _, name := range names {
		if remaining <= 0 {
			break
		}
		skill, err := backend.Get(ctx, name)
		if err != nil {
			log.Printf("[config-manager] resource skill unavailable name=%s err=%v", name, err)
			continue
		}
		content := strings.TrimSpace(skill.Content)
		if content == "" {
			continue
		}
		limit := configManagerResourceSkillMaxBytes
		if remaining < limit {
			limit = remaining
		}
		content, truncated := trimStringToUTF8Bytes(content, limit)
		if truncated {
			log.Printf("[config-manager] resource skill truncated name=%s limit=%d", name, limit)
		}
		if content == "" {
			continue
		}
		remaining -= len([]byte(content))
		loaded = append(loaded, agent.ConfigManagerResourceSkill{
			Name:        skill.Name,
			Description: skill.Description,
			Content:     content,
		})
	}
	if len(loaded) > 0 {
		loadedNames := make([]string, 0, len(loaded))
		for _, skill := range loaded {
			loadedNames = append(loadedNames, skill.Name)
		}
		log.Printf("[config-manager] loaded resource skills origin=%s names=%s", req.Origin, strings.Join(loadedNames, ","))
	}
	return loaded
}

func configManagerResourceSkillNames(req ConfigManagerRequest) []string {
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, existing := range out {
			if existing == name {
				return
			}
		}
		out = append(out, name)
	}

	origin := normalizeConfigManagerSignal(req.Origin)
	switch origin {
	case "lore":
		add(configManagerLoreSkill)
	case "automation", "automations":
		add(configManagerAutomationSkill)
	case "story_memory", "story-memory", "storymemory", "interactive_memory", "interactive-memory":
		add(configManagerStoryMemorySkill)
	case "teller", "tellers", "narrative", "style", "styles", "director", "story_director", "story-director", "story_directors", "story-directors", "actor_state", "actor-state", "actor_states", "actor-states", "memory_structure", "memory-structure", "memory_structures", "memory-structures", "story_memory_structure", "story-memory-structure", "story_memory_structures", "story-memory-structures":
		add(configManagerTellerSkill)
		add(configManagerStoryDirectorSkill)
		add(configManagerImagePresetSkill)
	case "image_preset", "image_preset_config", "image_presets", "image-preset", "image-presets", "preset", "presets":
		add(configManagerTellerSkill)
		add(configManagerImagePresetSkill)
	case "skills", "skill":
		add(configManagerSkillsSkill)
	case "agents", "agent":
		add(configManagerAgentConfigSkill)
	}

	signals := []string{req.Origin, req.ResourceID, req.StoryID, req.BranchID}
	for _, ref := range req.References {
		signals = append(signals, ref)
	}
	for key, value := range req.Context {
		signals = append(signals, key, value)
	}
	text := normalizeConfigManagerSignal(strings.Join(signals, " "))
	switch {
	case strings.Contains(text, "write_lore_items") || strings.Contains(text, "lore_item") || strings.Contains(text, "selected_lore") || strings.Contains(text, "资料库"):
		add(configManagerLoreSkill)
	}
	switch {
	case strings.Contains(text, "automation") || strings.Contains(text, "write_automations") || strings.Contains(text, "active_automation"):
		add(configManagerAutomationSkill)
	}
	switch {
	case strings.Contains(text, "story_memory") || strings.Contains(text, "story-memory") || strings.Contains(text, "storymemory") || strings.Contains(text, "selected_structure") || strings.Contains(text, "structure_id") || strings.Contains(text, "record_count"):
		add(configManagerStoryMemorySkill)
	}
	switch {
	case strings.Contains(text, "teller") || strings.Contains(text, "narrative") || strings.Contains(text, "叙事风格"):
		add(configManagerTellerSkill)
	}
	switch {
	case strings.Contains(text, "story_director") || strings.Contains(text, "write_story_directors") || strings.Contains(text, "event_package") || strings.Contains(text, "event-packages") || strings.Contains(text, "actor_state") || strings.Contains(text, "actor_states") || strings.Contains(text, "memory_structure") || strings.Contains(text, "memory-structure") || strings.Contains(text, "story_memory_structure") || strings.Contains(text, "story-memory-structure") || strings.Contains(text, "故事导演") || strings.Contains(text, "导演策略") || strings.Contains(text, "事件包") || strings.Contains(text, "事件系统") || strings.Contains(text, "状态系统") || strings.Contains(text, "结构化状态") || strings.Contains(text, "记忆结构") || strings.Contains(text, "trpg"):
		add(configManagerStoryDirectorSkill)
	}
	switch {
	case strings.Contains(text, "image_preset") || strings.Contains(text, "image_presets") || strings.Contains(text, "图像方案") || strings.Contains(text, "方案预设") || strings.Contains(text, "preset"):
		add(configManagerImagePresetSkill)
	}
	switch {
	case strings.Contains(text, "skills") || strings.Contains(text, "skill"):
		add(configManagerSkillsSkill)
	}
	switch {
	case strings.Contains(text, "agents") || strings.Contains(text, "agent_config") || strings.Contains(text, "subagent") || strings.Contains(text, "sub_agent"):
		add(configManagerAgentConfigSkill)
	}
	return out
}

func normalizeConfigManagerSignal(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func trimStringToUTF8Bytes(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return "", value != ""
	}
	if len([]byte(value)) <= limit {
		return value, false
	}
	used := 0
	for i, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = len(string(r))
		}
		if used+size > limit {
			return strings.TrimSpace(value[:i]), true
		}
		used += size
	}
	return value, false
}
