package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agent"
	novaskills "denova/internal/agent/skills"
)

const (
	// Resource Skills are loaded exactly up to these non-configurable safety
	// boundaries. The lower, user-configurable model injection limits belong to
	// the system-prompt composer so it can emit a visible truncation marker and
	// an original/included provenance receipt from the same source.
	configManagerResourceSkillMaxSourceBytes      = 512 * 1024
	configManagerResourceSkillMaxTotalSourceBytes = 3 * configManagerResourceSkillMaxSourceBytes

	configManagerAutomationSkill    = "automation-config"
	configManagerTellerSkill        = "teller-config"
	configManagerStoryDirectorSkill = "story-director-config"
	configManagerImagePresetSkill   = "image-preset-config"
	configManagerSkillsSkill        = "skills-creator"
	configManagerAgentConfigSkill   = "agent-config"
	configManagerLoreSkill          = "lore"
)

func loadConfigManagerResourceSkills(ctx context.Context, cfg *config.Config, req ConfigManagerRequest) ([]agent.ConfigManagerResourceSkill, error) {
	names := configManagerResourceSkillNames(req)
	if len(names) == 0 || cfg == nil {
		return nil, nil
	}
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		config.AgentKindConfigManager,
		config.ResolveAgentSkillOverrides(cfg, config.AgentKindConfigManager),
	)
	loaded := make([]agent.ConfigManagerResourceSkill, 0, len(names))
	totalSourceBytes := 0
	for _, name := range names {
		skill, err := backend.Get(ctx, name)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("[config-manager] resource skill unavailable name=%s err=%v", name, err)
			continue
		}
		content := strings.TrimSpace(skill.Content)
		if content == "" {
			continue
		}
		sourceBytes := len(content)
		if sourceBytes > configManagerResourceSkillMaxSourceBytes {
			return nil, fmt.Errorf(
				"resource Skill exceeds hard source limit / 配置 Skill 超过加载硬上限: name=%s bytes=%d limit=%d",
				name, sourceBytes, configManagerResourceSkillMaxSourceBytes,
			)
		}
		if sourceBytes > configManagerResourceSkillMaxTotalSourceBytes-totalSourceBytes {
			return nil, fmt.Errorf(
				"resource Skills exceed hard total source limit / 配置 Skills 超过加载总硬上限: next_name=%s loaded_bytes=%d next_bytes=%d limit=%d",
				name, totalSourceBytes, sourceBytes, configManagerResourceSkillMaxTotalSourceBytes,
			)
		}
		totalSourceBytes += sourceBytes
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
	return loaded, nil
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
	case "teller", "tellers", "narrative", "style", "styles", "director", "story_director", "story-director", "story_directors", "story-directors", "actor_state", "actor-state", "actor_states", "actor-states":
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
	case strings.Contains(text, "teller") || strings.Contains(text, "narrative") || strings.Contains(text, "叙事风格"):
		add(configManagerTellerSkill)
	}
	switch {
	case strings.Contains(text, "story_director") || strings.Contains(text, "write_story_directors") || strings.Contains(text, "event_package") || strings.Contains(text, "event-packages") || strings.Contains(text, "actor_state") || strings.Contains(text, "actor_states") || strings.Contains(text, "故事导演") || strings.Contains(text, "导演策略") || strings.Contains(text, "事件包") || strings.Contains(text, "事件系统") || strings.Contains(text, "状态系统") || strings.Contains(text, "结构化状态") || strings.Contains(text, "trpg"):
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
