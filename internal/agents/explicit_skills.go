package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	novaskills "denova/internal/agents/skills"
)

// ExplicitSkillInvocation is a Skill deterministically resolved from an
// explicit /<skill-name> mention in the current user message.
type ExplicitSkillInvocation struct {
	Name          string
	Instructions  string
	BaseDirectory string
}

// ExplicitSkillResolver is the optional conversation capability used by the
// shared runtime before its first model call. Implementations must only return
// Skills enabled for the current Agent.
type ExplicitSkillResolver interface {
	ResolveExplicitSkills(ctx context.Context, message string) ([]ExplicitSkillInvocation, error)
}

// ResolveExplicitSkillInvocations resolves active Agent Skills from one
// runtime configuration. Unknown slash commands remain ordinary user text.
func ResolveExplicitSkillInvocations(ctx context.Context, cfg *config.Config, agentKind, message string) ([]ExplicitSkillInvocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg == nil || !config.ResolveAgentTools(cfg, agentKind).Allows(config.AgentToolSkills) {
		return nil, nil
	}
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		config.ResolveAgentSkillOverrides(cfg, agentKind),
	)
	maxFragmentBytes := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	resolved := backend.ResolveExplicitInvocations(ctx, message)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	invocations := make([]ExplicitSkillInvocation, 0, len(resolved))
	for _, skill := range resolved {
		notice := explicitSkillRuntimeNotice(skill.Name)
		instructionsLimit := maxFragmentBytes - len(notice) - 2
		instructions := novaskills.FormatForModel(skill, instructionsLimit)
		if instructionsLimit <= 0 || !strings.HasPrefix(instructions, "# Skill: "+skill.Name) {
			return nil, fmt.Errorf("显式 Skill %q 无法放入当前单片段上下文上限 / explicit Skill %q cannot fit the configured context fragment limit", skill.Name, skill.Name)
		}
		invocations = append(invocations, ExplicitSkillInvocation{
			Name:          skill.Name,
			Instructions:  instructions,
			BaseDirectory: skill.BaseDirectory,
		})
	}
	return invocations, nil
}

func explicitSkillFragments(invocations []ExplicitSkillInvocation) []agentcontext.Fragment {
	fragments := make([]agentcontext.Fragment, 0, len(invocations))
	for index, invocation := range invocations {
		name := strings.TrimSpace(invocation.Name)
		if name == "" || strings.TrimSpace(invocation.Instructions) == "" {
			continue
		}
		fragment := turnFragment(
			fmt.Sprintf("turn_explicit_skill_%d_%s", index+1, name),
			"turn.skill.explicit",
			"显式 Skill / Explicit Skill: "+name,
			"apply a Skill explicitly selected by the user before the first model call",
			explicitSkillModelContent(name, invocation.Instructions),
			0,
		)
		fragment.Note = "source=active Skill catalog; explicit user invocation; base_directory=" + strings.TrimSpace(invocation.BaseDirectory)
		fragments = append(fragments, fragment)
	}
	return fragments
}

func explicitSkillModelContent(name, instructions string) string {
	return explicitSkillRuntimeNotice(name) + "\n\n" + strings.TrimSpace(instructions)
}

func explicitSkillRuntimeNotice(name string) string {
	return "[Denova runtime] 用户已显式指定 /" + name + "；运行时已在首轮模型请求前加载该 Skill。" +
		"不要仅为了再次读取同一 Skill 而调用 `skill` 工具；直接遵循下列说明继续处理。"
}

func validateExplicitSkillProjection(result agentcontext.Result, invocations []ExplicitSkillInvocation) error {
	if len(invocations) == 0 {
		return nil
	}
	byID := make(map[string]agentcontext.Fragment, len(result.Fragments))
	for _, fragment := range result.Fragments {
		byID[fragment.ID] = fragment
	}
	for index, invocation := range invocations {
		id := fmt.Sprintf("turn_explicit_skill_%d_%s", index+1, strings.TrimSpace(invocation.Name))
		fragment, ok := byID[id]
		expected := strings.TrimSpace(explicitSkillModelContent(invocation.Name, invocation.Instructions))
		if !ok || !fragment.Included || fragment.Truncated || strings.TrimSpace(fragment.Content) != expected {
			return fmt.Errorf("显式 Skill %q 未能完整进入模型上下文，请提高 Agent 上下文上限或减少本轮 Skill 数量 / explicit Skill %q did not fit the model context; raise the Agent context limits or request fewer Skills", invocation.Name, invocation.Name)
		}
	}
	return nil
}

func (r *chatRun) emitExplicitSkillLoads(invocations []ExplicitSkillInvocation) {
	if len(invocations) == 0 {
		return
	}
	meta := agentEventMetadata{
		AgentKind: r.options.AgentKind, RunID: r.runID,
		AgentName: r.options.RootAgentName, RootAgentName: r.options.RootAgentName,
	}
	if r.options.RootAgentName != "" {
		meta.RunPath = []string{r.options.RootAgentName}
	}
	for index, invocation := range invocations {
		args, err := json.Marshal(map[string]string{"name": invocation.Name})
		if err != nil {
			r.logger.ErrorContext(r.ctx, "explicit_skill_args_failed", slog.String("skill", invocation.Name), slog.String("error_class", safeErrorClass(err.Error())))
			continue
		}
		id := fmt.Sprintf("%s-explicit-skill-%02d", firstNonEmpty(r.runID, "run"), index+1)
		r.emit(Event{Type: "tool_call", Data: meta.appendTo(map[string]interface{}{
			"id": id, "name": "skill", "args": string(args),
		})})
		r.emit(Event{Type: "tool_result", Data: meta.appendTo(map[string]interface{}{
			"id": id, "name": "skill", "content": invocation.Instructions,
		})})
		r.logger.InfoContext(r.ctx, "explicit_skill_loaded", slog.String("skill", invocation.Name), slog.String("base_directory", invocation.BaseDirectory))
	}
}
