package app

import (
	"context"
	"log"
	"strings"
	"unicode/utf8"

	"nova/config"
	"nova/internal/agent"
	novaskills "nova/internal/skills"
)

const (
	configManagerResourceSkillMaxBytes      = 12 * 1024
	configManagerResourceSkillMaxTotalBytes = 32 * 1024

	configManagerAutomationSkill  = "automation-config"
	configManagerStoryMemorySkill = "story-memory-config"
	configManagerTellerSkill      = "teller-config"
	configManagerSkillsSkill      = "skills-creator"
	configManagerAgentConfigSkill = "agent-config"
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
	case "automation", "automations":
		add(configManagerAutomationSkill)
	case "story_memory", "story-memory", "storymemory", "interactive_memory", "interactive-memory":
		add(configManagerStoryMemorySkill)
	case "teller", "tellers", "director", "narrative":
		add(configManagerTellerSkill)
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
	case strings.Contains(text, "automation") || strings.Contains(text, "write_automations") || strings.Contains(text, "active_automation"):
		add(configManagerAutomationSkill)
	}
	switch {
	case strings.Contains(text, "story_memory") || strings.Contains(text, "story-memory") || strings.Contains(text, "storymemory") || strings.Contains(text, "selected_structure") || strings.Contains(text, "structure_id") || strings.Contains(text, "record_count"):
		add(configManagerStoryMemorySkill)
	}
	switch {
	case strings.Contains(text, "teller") || strings.Contains(text, "director") || strings.Contains(text, "narrative"):
		add(configManagerTellerSkill)
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
