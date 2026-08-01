package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agents/prompts"
	novaskills "denova/internal/agents/skills"
)

const (
	// Resource Skills are loaded exactly up to these non-configurable safety
	// boundaries. The lower, user-configurable model injection limits belong to
	// the system-prompt composer so it can emit a visible truncation marker and
	// an original/included provenance receipt from the same source.
	configManagerResourceSkillMaxSourceBytes      = 512 * 1024
	configManagerResourceSkillMaxTotalSourceBytes = configManagerResourceSkillMaxSourceBytes
	configManagerSkillName                        = "config-manager"
)

func loadConfigManagerResourceSkills(ctx context.Context, cfg *config.Config, req ConfigManagerRequest) ([]prompts.ConfigManagerResourceSkill, error) {
	names := configManagerResourceSkillNames(req)
	if len(names) == 0 || cfg == nil {
		return nil, nil
	}
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		config.AgentKindConfigManager,
		config.ResolveAgentSkillOverrides(cfg, config.AgentKindConfigManager),
	)
	loaded := make([]prompts.ConfigManagerResourceSkill, 0, len(names))
	totalSourceBytes := 0
	for _, name := range names {
		skill, err := backend.Get(ctx, name)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			slog.WarnContext(ctx, fmt.Sprintf("[config-manager] resource skill unavailable name=%s err=%v", name, err))
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
		loaded = append(loaded, prompts.ConfigManagerResourceSkill{
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
		slog.InfoContext(ctx, fmt.Sprintf("[config-manager] loaded resource skills origin=%s names=%s", req.Origin, strings.Join(loadedNames, ",")))
	}
	return loaded, nil
}

func configManagerResourceSkillNames(req ConfigManagerRequest) []string {
	_ = req
	return []string{configManagerSkillName}
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
