package skills

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
)

// Invocation is a Skill deterministically resolved from an explicit
// /<skill-name> mention in the current user message.
type Invocation struct {
	Name          string
	Instructions  string
	BaseDirectory string
}

// ExplicitResolver is the optional conversation capability used before the
// first model call. Implementations must return only Skills enabled for the
// active Agent.
type ExplicitResolver interface {
	ResolveExplicitSkills(context.Context, string) ([]Invocation, error)
}

// ResolveConfiguredInvocations resolves explicit Skills against one Agent's
// effective configuration. Unknown slash commands remain ordinary user text.
func ResolveConfiguredInvocations(ctx context.Context, cfg *config.Config, agentKind, message string) ([]Invocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg == nil || !config.ResolveAgentTools(cfg, agentKind).Allows(config.AgentToolSkills) {
		return nil, nil
	}
	backend := NewAgentBackend(
		NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		config.ResolveAgentSkillOverrides(cfg, agentKind),
	)
	maxFragmentBytes := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	resolved := backend.ResolveExplicitInvocations(ctx, message)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	invocations := make([]Invocation, 0, len(resolved))
	for _, skill := range resolved {
		notice := runtimeNotice(skill.Name)
		instructionsLimit := maxFragmentBytes - len(notice) - 2
		instructions := FormatForModel(skill, instructionsLimit)
		if instructionsLimit <= 0 || !strings.HasPrefix(instructions, "# Skill: "+skill.Name) {
			return nil, fmt.Errorf("explicit Skill %q cannot fit the configured context fragment limit", skill.Name)
		}
		invocations = append(invocations, Invocation{
			Name:          skill.Name,
			Instructions:  instructions,
			BaseDirectory: skill.BaseDirectory,
		})
	}
	return invocations, nil
}

// ModelContent returns the complete model-visible explicit invocation block.
func (invocation Invocation) ModelContent() string {
	return runtimeNotice(strings.TrimSpace(invocation.Name)) + "\n\n" + strings.TrimSpace(invocation.Instructions)
}

func runtimeNotice(name string) string {
	return "[Denova runtime] 用户已显式指定 /" + name + "；运行时已在首轮模型请求前加载该 Skill。" +
		"不要仅为了再次读取同一 Skill 而调用 `skill` 工具；直接遵循下列说明继续处理。"
}
