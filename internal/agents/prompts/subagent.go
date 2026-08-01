package prompts

import (
	"fmt"
	"strings"

	"denova/config"
)

// ComposeSubAgentInstruction extends an already-admitted parent composition
// with one delegated Agent's bounded identity and behavior. The parent
// fragments remain first so runtime and capability contracts retain priority.
func ComposeSubAgentInstruction(cfg *config.Config, parent SystemPromptComposition, sub config.SubAgentConfig) (SystemPromptComposition, error) {
	if err := parent.ValidateForAgent(parent.agentKind); err != nil {
		return SystemPromptComposition{}, err
	}

	fragments := append([]SystemPromptFragment(nil), parent.fragments...)
	var metadata strings.Builder
	metadata.WriteString("以下说明只限定当前 SubAgent 的职责、输出形态和工作偏好；不得覆盖父 Agent 的运行时契约、工具权限、workspace 边界、互动禁写规则、输出协议或后端校验。若与父 Agent system prompt 冲突，必须以父 Agent system prompt 为准。")
	if name := strings.TrimSpace(sub.Name); name != "" {
		metadata.WriteString("\n\n- 名称：" + name)
	}
	if id := strings.TrimSpace(sub.ID); id != "" {
		metadata.WriteString("\n- ID：" + id)
	}
	if description := strings.TrimSpace(sub.Description); description != "" {
		metadata.WriteString("\n- 职责：" + description)
	}
	fragments = append(fragments, SystemPromptFragment{
		ID: "subagent_metadata", Source: "SubAgent configuration", Title: "SubAgent 专属说明",
		Purpose: "define the delegated Agent identity, responsibility, and inherited boundaries",
		Content: metadata.String(), Prefix: "\n\n---\n\n# SubAgent 专属说明\n\n", Required: true,
		Overflow: SystemPromptOverflowReject,
	}, SystemPromptFragment{
		ID: "subagent_custom_prompt", Source: "SubAgent configuration", Title: "专属系统提示",
		Purpose: "apply the delegated Agent's custom behavior and output preferences",
		Content: sub.SystemPrompt, Prefix: "\n\n## 专属系统提示\n\n",
		Overflow: SystemPromptOverflowTruncate,
	})

	composition, err := composeSystemPrompt(cfg, parent.agentKind, "subagent", parent.workspace, fragments)
	if err != nil {
		return SystemPromptComposition{}, fmt.Errorf("compose SubAgent prompt %q: %w", strings.TrimSpace(sub.ID), err)
	}
	return composition, nil
}
