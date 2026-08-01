package chat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	novaskills "denova/internal/agents/skills"
)

func explicitSkillFragments(invocations []novaskills.Invocation) []agentcontext.Fragment {
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
			invocation.ModelContent(),
			0,
		)
		fragment.Note = "source=active Skill catalog; explicit user invocation; base_directory=" + strings.TrimSpace(invocation.BaseDirectory)
		fragments = append(fragments, fragment)
	}
	return fragments
}

func validateExplicitSkillProjection(result agentcontext.Result, invocations []novaskills.Invocation) error {
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
		expected := strings.TrimSpace(invocation.ModelContent())
		if !ok || !fragment.Included || fragment.Truncated || strings.TrimSpace(fragment.Content) != expected {
			return fmt.Errorf("显式 Skill %q 未能完整进入模型上下文，请提高 Agent 上下文上限或减少本轮 Skill 数量 / explicit Skill %q did not fit the model context; raise the Agent context limits or request fewer Skills", invocation.Name, invocation.Name)
		}
	}
	return nil
}

func (r *chatRun) emitExplicitSkillLoads(invocations []novaskills.Invocation) {
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
			r.logger.ErrorContext(r.ctx, "explicit_skill_args_failed", slog.String("skill", invocation.Name), slog.String("error_class", agentrun.ErrorClass(err.Error())))
			continue
		}
		id := fmt.Sprintf("%s-explicit-skill-%02d", firstNonEmpty(r.runID, "run"), index+1)
		r.emit(agentrun.Event{Type: "tool_call", Data: meta.appendTo(map[string]interface{}{
			"id": id, "name": "skill", "args": string(args),
		})})
		r.emit(agentrun.Event{Type: "tool_result", Data: meta.appendTo(map[string]interface{}{
			"id": id, "name": "skill", "content": invocation.Instructions,
		})})
		r.logger.InfoContext(r.ctx, "explicit_skill_loaded", slog.String("skill", invocation.Name), slog.String("base_directory", invocation.BaseDirectory))
	}
}
