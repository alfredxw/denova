package agents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/book"
	"denova/internal/prompts"
)

// IDEStoryTeller describes the resolved writing-director rules used by one
// Agent run. Runtime workspace state is projected separately at turn time.
type IDEStoryTeller struct {
	ID                      string
	Name                    string
	Description             string
	Prompt                  string
	StyleRules              []StyleRule
	ImagePresetID           string
	ImagePresetName         string
	ImagePresetSystemPrompt string
}

// ConfigManagerResourceSkill is a bounded, already-resolved Skill body that
// config_manager should treat as run-scoped schema/workflow guidance.
type ConfigManagerResourceSkill struct {
	Name        string
	Description string
	Content     string
}

// ComposeGeneralInstruction assembles the project-scoped contract for the
// General Agent. The project directory is its working root; Denova's user data
// directory receives no implicit privilege or restriction when explicitly
// added as a project.
func ComposeGeneralInstruction(cfg *config.Config) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = strings.TrimSpace(cfg.Workspace)
	}
	return composeBuiltinSystemInstruction(
		cfg,
		config.AgentKindGeneral,
		"general",
		workspace,
		"builtin_base",
		"General Agent 工作规则",
		"define the project-scoped general-purpose agent workflow",
		strings.Join([]string{
			"你是 Denova 的 General Agent，定位接近 Codex、Claude Code 或 OMP：在用户明确添加的 Project 中提供通用研究、开发、写作、整理和自动化服务。",
			"当前 Project 目录就是工作根目录。先理解用户目标和现有结构，再选择最小且可验证的操作；任务需要时可以读取、创建、编辑文件并执行命令。",
			"开始较大任务前，按需检查根目录及目标路径附近的 AGENTS.md、CLAUDE.md、README、贡献指南等项目说明，并遵守离目标文件最近的适用规则。不要为了发现规则而无界遍历或一次性注入完整目录树。",
			"文件发现与内容搜索默认遵守 .gitignore；用户明确指定路径、直接 read/write/edit 的文件不因 .gitignore 被硬屏蔽；shell 命令保持其原生语义。不要自动创建或修改 .gitignore。",
			"Denova 不会因为某个 Project 恰好指向 Denova 数据目录而施加额外的隐藏限制；它与用户显式添加的其他目录一视同仁，仍受工具权限、工作根目录和用户指令约束。",
			"完成变更后进行与风险相称的检查，并清楚说明做了什么、验证了什么以及仍存在的限制。",
		}, "\n\n"),
	)
}

// ComposeInstruction assembles and admits the exact IDE system instruction.
func ComposeInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) (SystemPromptComposition, error) {
	workspace, creator, _ := idePromptWorkspaceSources(cfg, state)
	builtIn := make([]SystemPromptFragment, 0, 4+len(teller.StyleRules))
	builtIn = append(builtIn, creatorSystemPromptFragment(creator))
	builtIn = append(builtIn, tellerSystemPromptFragment("ide_teller", "写作模式默认导演规则", teller.ID, teller.Name, teller.Description, teller.Prompt))
	builtIn = append(builtIn, styleRuleSystemPromptFragments(teller.StyleRules)...)
	builtIn = append(builtIn, SystemPromptFragment{
		ID: "builtin_base", Source: "Denova built-in", Title: "写作模式流程配置",
		Purpose: "define the built-in writing workflow and workspace rules",
		Content: ideFlowInstruction(cfg, workspace), Required: true, Overflow: SystemPromptOverflowReject,
	})
	if prompt := strings.TrimSpace(teller.ImagePresetSystemPrompt); prompt != "" {
		var content strings.Builder
		content.WriteString("## 图像方案系统规则（仅用于图像生成）\n\n")
		if id := strings.TrimSpace(teller.ImagePresetID); id != "" {
			content.WriteString("- id: " + id + "\n")
		}
		if name := strings.TrimSpace(teller.ImagePresetName); name != "" {
			content.WriteString("- name: " + name + "\n")
		}
		content.WriteString("\n以下规则只在构造 `generate_image` 的图像提示词时生效；普通正文写作、资料库修改和非图像任务不要套用这些视觉约束。\n\n")
		content.WriteString(prompt)
		builtIn = append(builtIn, SystemPromptFragment{
			ID: "image_preset", Source: "image preset configuration",
			Title: "图像方案系统规则", Purpose: "constrain image prompts generated during writing tasks",
			Content: content.String(), Overflow: SystemPromptOverflowTruncate,
		})
	}
	return composeProtectedSystemInstruction(cfg, config.AgentKindIDE, "ide", workspace, builtIn)
}

// BuildInstructionComposition preserves the existing display API while
// retaining any admission error on the returned artifact.
func BuildInstructionComposition(cfg *config.Config, state *book.State, teller IDEStoryTeller) SystemPromptComposition {
	composition, err := ComposeInstruction(cfg, state, teller)
	if err != nil {
		workspace, _, _ := idePromptWorkspaceSources(cfg, state)
		return failedSystemPromptComposition("ide", config.AgentKindIDE, workspace, err)
	}
	return composition
}

// ComposeInteractiveStoryInstruction assembles and admits the exact game
// narrator instruction without silently pre-truncating style sources.
func ComposeInteractiveStoryInstruction(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	creator := ""
	if state != nil {
		creator = state.ReadCreatorPrompt()
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	builtIn := make([]SystemPromptFragment, 0, 4+len(teller.StyleRules))
	builtIn = append(builtIn, creatorSystemPromptFragment(creator))
	builtIn = append(builtIn, tellerSystemPromptFragment(
		"interactive_teller", "导演系统规则", teller.StoryTellerID, teller.StoryTellerName,
		teller.StoryTellerDescription, teller.StoryTellerSystemPrompt,
	))
	builtIn = append(builtIn, styleRuleSystemPromptFragments(teller.StyleRules)...)
	baseInput := teller
	baseInput.CreatorPrompt = ""
	baseInput.StoryTellerSystemPrompt = ""
	baseInput.StyleRules = nil
	baseInput.Workspace = workspace
	if baseInput.ReplyTargetChars <= 0 && cfg != nil {
		baseInput.ReplyTargetChars = cfg.InteractiveReplyTargetChars
	}
	builtIn = append(builtIn, SystemPromptFragment{
		ID: "builtin_base", Source: "Denova built-in", Title: "互动故事流程规则",
		Purpose: "define the built-in game narration workflow and output behavior",
		Content: prompts.BuildInteractiveStorySystemInstruction(baseInput), Required: true, Overflow: SystemPromptOverflowReject,
	})
	return composeProtectedSystemInstruction(cfg, config.AgentKindInteractiveStory, "interactive", workspace, builtIn)
}

// BuildInteractiveStoryInstructionComposition preserves the display API.
func BuildInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) SystemPromptComposition {
	composition, err := ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("interactive", config.AgentKindInteractiveStory, workspace, err)
	}
	return composition
}

// ComposeInteractiveDirectorInstruction assembles the exact background
// director instruction used by both execution and context analysis.
func ComposeInteractiveDirectorInstruction(cfg *config.Config, state *book.State) (SystemPromptComposition, error) {
	return composeBuiltinSystemInstruction(cfg, config.AgentKindInteractiveDirector, "interactive_director", workspaceForPrompt(cfg, state), "builtin_base", "后台导演系统规则", "define the interactive director planning workflow", prompts.BuildInteractiveDirectorSystemInstruction())
}

// ComposeConfigManagerInstruction assembles the config manager prompt and
// gives every auto-loaded Skill its own independently bounded source receipt.
func ComposeConfigManagerInstruction(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) (SystemPromptComposition, error) {
	workspace := ""
	creator := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	if state != nil {
		if workspace == "" {
			workspace = state.Workspace()
		}
		creator = state.ReadCreatorPrompt()
	}
	builtIn := []SystemPromptFragment{creatorSystemPromptFragment(creator), {
		ID: "builtin_base", Source: "Denova built-in", Title: "配置管理 Agent 内置规则",
		Purpose: "define resource configuration workflows and safety boundaries",
		Content: configManagerFlowInstructionFor(workspace, ""), Required: true, Overflow: SystemPromptOverflowReject,
	}}
	skillOrdinal := 0
	skillOccurrences := make(map[string]int, len(resourceSkills))
	for _, skill := range resourceSkills {
		name := strings.TrimSpace(skill.Name)
		content := strings.TrimSpace(skill.Content)
		if name == "" || content == "" {
			continue
		}
		skillOrdinal++
		skillOccurrences[name]++
		var skillContent strings.Builder
		skillContent.WriteString("### /")
		skillContent.WriteString(name)
		skillContent.WriteString("\n\n")
		if description := strings.TrimSpace(skill.Description); description != "" {
			skillContent.WriteString("description: ")
			skillContent.WriteString(description)
			skillContent.WriteString("\n\n")
		}
		skillContent.WriteString(content)
		prefix := "\n\n## 配置管理 Skill\n\n以下内容来自当前生效的 config-manager Skill。资源细节位于 references；按需使用 read 读取，不要把全部参考一次性注入上下文。若与运行时契约或后端校验冲突，以运行时契约和后端校验为准。\n\n"
		if skillOrdinal > 1 {
			prefix = "\n"
		}
		builtIn = append(builtIn, SystemPromptFragment{
			ID: fmt.Sprintf("config_skill:%s:%03d", shortSystemPromptSHA(systemPromptSHA(name)), skillOccurrences[name]), Source: "配置 Skill",
			Title: "/" + name, Purpose: "provide run-scoped configuration schema and workflow guidance",
			Content: skillContent.String(), Prefix: prefix, Suffix: "\n", Overflow: SystemPromptOverflowTruncate,
		})
	}
	return composeProtectedSystemInstruction(cfg, config.AgentKindConfigManager, "config_manager", workspace, builtIn)
}

func BuildConfigManagerInstructionComposition(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) SystemPromptComposition {
	composition, err := ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("config_manager", config.AgentKindConfigManager, workspace, err)
	}
	return composition
}

// ComposeImageInstruction assembles image runtime rules, CREATOR.md, and the
// caller's image prompt as independently admitted sources.
func ComposeImageInstruction(cfg *config.Config, state *book.State, systemPrompt string) (SystemPromptComposition, error) {
	base, workspace, creator := buildImageBuiltinInstruction(cfg, state, "")
	builtIn := []SystemPromptFragment{creatorSystemPromptFragment(creator), {
		ID: "builtin_base", Source: "Denova built-in", Title: "图像 Agent 基础规则",
		Purpose: "define the built-in image generation workflow", Content: base,
		Required: true, Overflow: SystemPromptOverflowReject,
	}, {
		ID: "image_call_prompt", Source: "image generation caller", Title: "调用点系统提示",
		Purpose: "constrain the current image generation request", Content: systemPrompt,
		Prefix: "\n\n## 调用点系统提示\n\n", Overflow: SystemPromptOverflowTruncate,
	}}
	return composeProtectedSystemInstruction(cfg, config.AgentKindImage, "image", workspace, builtIn)
}

func BuildImageInstructionComposition(cfg *config.Config, state *book.State, systemPrompt string) SystemPromptComposition {
	composition, err := ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("image", config.AgentKindImage, workspace, err)
	}
	return composition
}

func BuildImageInstruction(cfg *config.Config, state *book.State, systemPrompt string) string {
	return BuildImageInstructionComposition(cfg, state, systemPrompt).Instruction()
}

func creatorSystemPromptFragment(creator string) SystemPromptFragment {
	return SystemPromptFragment{
		ID: "creator", Source: "workspace CREATOR.md", Title: "创作者指令",
		Purpose: "apply stable workspace-level creative constraints", Content: creator,
		Prefix: "# 创作者指令（最高优先级）\n\n", Suffix: "\n\n---\n\n",
		Overflow: SystemPromptOverflowTruncate,
	}
}

func tellerSystemPromptFragment(id, title, tellerID, name, description, content string) SystemPromptFragment {
	if strings.TrimSpace(tellerID) == "" && strings.TrimSpace(name) == "" && strings.TrimSpace(description) == "" && strings.TrimSpace(content) == "" {
		return SystemPromptFragment{
			ID: id, Source: "story teller configuration", Title: title,
			Purpose: "apply the selected story teller behavior and style", Overflow: SystemPromptOverflowTruncate,
		}
	}
	var visible strings.Builder
	visible.WriteString("# " + title + "\n\n")
	if value := strings.TrimSpace(tellerID); value != "" {
		visible.WriteString("导演 ID: " + value + "\n")
	}
	if value := strings.TrimSpace(name); value != "" {
		visible.WriteString("导演名称: " + value + "\n")
	}
	if value := strings.TrimSpace(description); value != "" {
		visible.WriteString("导演说明: " + value + "\n")
	}
	visible.WriteString("\n")
	visible.WriteString(strings.TrimSpace(content))
	visible.WriteString("\n\n以上导演规则只用于当前 Agent 的创作与叙事职责；不得覆盖运行时契约、工具边界或用户本轮明确要求。")
	return SystemPromptFragment{
		ID: id, Source: "story teller configuration", Title: title,
		Purpose: "apply the selected story teller behavior and style", Content: visible.String(),
		Suffix:   "\n\n---\n\n",
		Overflow: SystemPromptOverflowTruncate,
	}
}

func styleRuleSystemPromptFragments(rules []StyleRule) []SystemPromptFragment {
	entries := make([]SystemPromptFragment, 0, len(rules))
	ordinal := 0
	occurrences := make(map[string]int, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		ordinal++
		content := strings.TrimSpace(prompts.StyleRuleEntryInstruction(rule, ordinal))
		if content == "" {
			continue
		}
		title := "文风参考：全局"
		identity := "global"
		if !rule.Global {
			title = "文风参考：" + scene
			identity = "scene:" + scene
		}
		occurrences[identity]++
		entries = append(entries, SystemPromptFragment{
			ID: fmt.Sprintf("style_rule:%s:%03d", shortSystemPromptSHA(systemPromptSHA(identity)), occurrences[identity]), Source: "current narrative style", Title: title,
			Purpose: "provide optional prose style references for the selected scene",
			Content: content, Suffix: "\n", Overflow: SystemPromptOverflowTruncate,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	fragments := make([]SystemPromptFragment, 0, len(entries)+2)
	fragments = append(fragments, SystemPromptFragment{
		ID: "style_protocol_header", Source: "Denova built-in", Title: "文风参考协议",
		Purpose: "describe the provenance and shape of the following style entries",
		Content: prompts.StyleRulesProtocolHeader(), Required: true, Overflow: SystemPromptOverflowReject,
	})
	fragments = append(fragments, entries...)
	fragments = append(fragments, SystemPromptFragment{
		ID: "style_protocol_footer", Source: "Denova built-in", Title: "文风参考触发规则",
		Purpose: "define when and how style references may influence generated prose",
		Content: prompts.StyleRulesProtocolFooter(), Suffix: "\n\n---\n\n", Required: true, Overflow: SystemPromptOverflowReject,
	})
	return fragments
}

func idePromptWorkspaceSources(cfg *config.Config, state *book.State) (workspace, creator, stateContext string) {
	if cfg != nil {
		workspace = cfg.Workspace
	}
	if state != nil {
		creator = state.ReadCreatorPrompt()
		stateContext = state.CompactContext()
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	return workspace, creator, stateContext
}

func (c SystemPromptComposition) logForRun(options RunOptions) {
	if c.isZero() {
		return
	}
	if c.assemblyErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-prompt] system composition failed mode=%s workspace=%s task_id=%s session_id=%s err=%v", c.mode, c.workspace, options.TaskID, options.SessionID, c.assemblyErr))
		return
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[agent-prompt] system composition mode=%s workspace=%s task_id=%s session_id=%s bytes=%d sha=%s sources=%s",
		c.mode, c.workspace, options.TaskID, options.SessionID, c.injectedBytes, c.instructionHash, systemPromptManifestSummary(c.manifest),
	))
}

func systemPromptManifestSummary(manifest []SystemPromptManifestEntry) string {
	parts := make([]string, 0, len(manifest))
	for _, entry := range manifest {
		parts = append(parts, fmt.Sprintf(
			"id=%q,source=%q,title=%q,bytes=%d/%d,sha=%s,included=%t,truncated=%t,rejected=%t,reason=%q",
			entry.ID, entry.Source, entry.Title, entry.IncludedBytes, entry.OriginalBytes, shortSystemPromptSHA(entry.IncludedSHA),
			entry.Included, entry.Truncated, entry.Rejected, entry.Reason,
		))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(parts), strings.Join(parts, "; "))
}

func shortSystemPromptSHA(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
