package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
)

// IDEStoryTeller 描述写作 Agent 本轮使用的默认导演规则。
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

// BuildInstruction 构建写作 Agent 的稳定系统指令；动态作品状态由会话运行时追加。
// 实际的 Prompt 文本集中在 internal/prompts 包，这里只负责把 cfg/state 翻译成 prompts.SystemInstructionInput。
func BuildInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) string {
	return BuildInstructionComposition(cfg, state, teller).Instruction()
}

// BuildInstructionComposition returns the IDE system prompt and its auditable source summary.
func BuildInstructionComposition(cfg *config.Config, state *book.State, teller IDEStoryTeller) SystemPromptCompositionLog {
	teller.StyleRules = boundedStyleRules(teller.StyleRules, maxStyleRuleContextChars)
	builtIn, workspace, creator, stateContext := buildIDEBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindIDE, builtIn)
	var stateParts []book.CompactContextPart
	if state != nil {
		stateParts = state.CompactContextParts()
	}
	extraSources := []promptSource{{
		source:  "系统提示",
		title:   "写作模式默认导演规则",
		content: teller.Prompt,
		note:    teller.ID,
	}}
	if strings.TrimSpace(teller.ImagePresetSystemPrompt) != "" {
		title := "图像方案系统规则"
		if strings.TrimSpace(teller.ImagePresetName) != "" {
			title = "图像方案系统规则：" + strings.TrimSpace(teller.ImagePresetName)
		}
		extraSources = append(extraSources, promptSource{
			source:  "系统提示",
			title:   title,
			content: teller.ImagePresetSystemPrompt,
			note:    teller.ImagePresetID,
		})
	}
	extraSources = append(extraSources, styleRulePromptSources(teller.StyleRules)...)
	return SystemPromptCompositionLog{
		mode:         "ide",
		workspace:    workspace,
		creator:      creator,
		stateContext: stateContext,
		stateParts:   stateParts,
		instruction:  instruction,
		extraSources: extraSources,
	}
}

// SystemPromptCompositionLog records the source breakdown for one system prompt.
type SystemPromptCompositionLog struct {
	mode         string
	workspace    string
	creator      string
	stateContext string
	stateParts   []book.CompactContextPart
	instruction  string
	extraSources []promptSource
}

func (l SystemPromptCompositionLog) Instruction() string {
	return l.instruction
}

func (l SystemPromptCompositionLog) isZero() bool {
	return strings.TrimSpace(l.mode) == "" && strings.TrimSpace(l.instruction) == ""
}

func (l SystemPromptCompositionLog) logForRun(options RunOptions) {
	if l.isZero() {
		return
	}
	log.Printf(
		"[agent-prompt] system composition mode=%s workspace=%s task_id=%s session_id=%s creator=%s state=%s instruction=%s",
		l.mode,
		l.workspace,
		options.TaskID,
		options.SessionID,
		promptPartSummary(l.creator),
		promptPartSummary(l.stateContext),
		promptPartSummary(l.instruction),
	)
	log.Printf("[agent-prompt] system sources mode=%s workspace=%s task_id=%s session_id=%s sources=%s", l.mode, l.workspace, options.TaskID, options.SessionID, systemPromptSourceSummary(l.mode, l.creator, l.stateParts, l.extraSources...))
}

func newInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) SystemPromptCompositionLog {
	teller.StyleRules = boundedStyleRules(teller.StyleRules, maxStyleRuleContextChars)
	builtIn, workspace, creator := buildInteractiveStoryBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindInteractiveStory, builtIn)
	extraSources := []promptSource{{
		source:  "系统提示",
		title:   "导演系统规则",
		content: teller.StoryTellerSystemPrompt,
		note:    teller.StoryTellerID,
	}}
	extraSources = append(extraSources, styleRulePromptSources(teller.StyleRules)...)
	return SystemPromptCompositionLog{
		mode:         "interactive",
		workspace:    workspace,
		creator:      creator,
		instruction:  instruction,
		extraSources: extraSources,
	}
}

// BuildInteractiveStoryInstructionComposition returns the interactive story prompt and its source summary.
func BuildInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) SystemPromptCompositionLog {
	return newInteractiveStoryInstructionComposition(cfg, state, teller)
}

// BuildConfigManagerInstructionComposition returns the config manager prompt and its source summary.
func BuildConfigManagerInstructionComposition(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) SystemPromptCompositionLog {
	builtIn, workspace, creator := buildConfigManagerBuiltinInstruction(cfg, state)
	builtInWithSkills := appendConfigManagerResourceSkills(builtIn, resourceSkills)
	instruction := protectedSystemInstruction(cfg, config.AgentKindConfigManager, builtInWithSkills)
	extraSources := []promptSource{{
		source:  "系统提示",
		title:   "配置管理 Agent 内置规则",
		content: builtIn,
		note:    "tool-chain",
	}}
	for _, skill := range resourceSkills {
		if strings.TrimSpace(skill.Name) == "" || strings.TrimSpace(skill.Content) == "" {
			continue
		}
		extraSources = append(extraSources, promptSource{
			source:  "配置 Skill",
			title:   "/" + strings.TrimSpace(skill.Name),
			content: skill.Content,
			note:    strings.TrimSpace(skill.Description),
		})
	}
	return SystemPromptCompositionLog{
		mode:         "config_manager",
		workspace:    workspace,
		creator:      creator,
		instruction:  instruction,
		extraSources: extraSources,
	}
}

// BuildImageInstructionComposition returns the generic image Agent prompt and its source summary.
func BuildImageInstructionComposition(cfg *config.Config, state *book.State, systemPrompt string) SystemPromptCompositionLog {
	builtIn, workspace, creator := buildImageBuiltinInstruction(cfg, state, systemPrompt)
	instruction := protectedSystemInstruction(cfg, config.AgentKindImage, builtIn)
	extraSources := []promptSource{{
		source:  "系统提示",
		title:   "图像 Agent 调用点规则",
		content: systemPrompt,
		note:    "runtime",
	}}
	return SystemPromptCompositionLog{
		mode:         "image",
		workspace:    workspace,
		creator:      creator,
		instruction:  instruction,
		extraSources: extraSources,
	}
}

func BuildImageInstruction(cfg *config.Config, state *book.State, systemPrompt string) string {
	return BuildImageInstructionComposition(cfg, state, systemPrompt).Instruction()
}

func buildIDEBuiltinInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) (string, string, string, string) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	creator := ""
	stateContext := ""
	workspace := ""
	workspace = cfg.Workspace
	if state != nil {
		creator = state.ReadCreatorPrompt()
		stateContext = state.CompactContext()
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	builtIn := prompts.BuildSystemInstruction(prompts.SystemInstructionInput{
		CreatorPrompt:          creator,
		Workspace:              workspace,
		StateContext:           stateContext,
		StoryTellerID:          teller.ID,
		StoryTellerName:        teller.Name,
		StoryTellerDescription: teller.Description,
		StoryTellerPrompt:      teller.Prompt,
		StyleRules:             boundedStyleRules(teller.StyleRules, maxStyleRuleContextChars),
		ChapterFilenameFormat:  cfg.ChapterFilenameFormat,
		VolumeDirFormat:        cfg.VolumeDirFormat,
		ChapterGroupMin:        cfg.ChapterGroupMin,
		ChapterGroupMax:        cfg.ChapterGroupMax,
	})
	if imagePresetSystem := imagePresetSystemInstruction(teller); imagePresetSystem != "" {
		builtIn = strings.TrimSpace(builtIn) + "\n\n" + imagePresetSystem
	}
	return builtIn, workspace, creator, stateContext
}

func imagePresetSystemInstruction(teller IDEStoryTeller) string {
	prompt := strings.TrimSpace(teller.ImagePresetSystemPrompt)
	if prompt == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 图像方案系统规则（仅用于图像生成）\n\n")
	if id := strings.TrimSpace(teller.ImagePresetID); id != "" {
		sb.WriteString("- id: ")
		sb.WriteString(id)
		sb.WriteString("\n")
	}
	if name := strings.TrimSpace(teller.ImagePresetName); name != "" {
		sb.WriteString("- name: ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	sb.WriteString("\n以下规则只在构造 `generate_image` 的图像提示词时生效；普通正文写作、资料库修改和非图像任务不要套用这些视觉约束。\n\n")
	sb.WriteString(prompt)
	return strings.TrimSpace(sb.String())
}

const (
	ideWorkspaceStableContextTitle  = "稳定作品上下文"
	ideWorkspaceDynamicContextTitle = "本轮动态作品状态"
	ideContextMaxOpenFiles          = 20
	ideContextMaxPathRunes          = 240
)

type IDEWorkspaceRuntimeContexts struct {
	StableTitle  string
	Stable       string
	DynamicTitle string
	Dynamic      string
}

func IDEWorkspaceRuntimeContextsForState(state *book.State) IDEWorkspaceRuntimeContexts {
	contexts := IDEWorkspaceRuntimeContexts{
		StableTitle:  ideWorkspaceStableContextTitle,
		DynamicTitle: ideWorkspaceDynamicContextTitle,
	}
	if state == nil {
		return contexts
	}
	contexts.Stable = strings.TrimSpace(state.StableContext())
	contexts.Dynamic = strings.TrimSpace(state.DynamicContext())
	if contexts.Stable == "" && contexts.Dynamic == "" {
		contexts.Stable = prompts.EmptyIDEStateHint()
	}
	return contexts
}

func IDEWorkspaceRuntimeContextsForRequest(state *book.State, req ChatRequest) IDEWorkspaceRuntimeContexts {
	contexts := IDEWorkspaceRuntimeContextsForState(state)
	ideContext := IDEContextRuntimeContext(req.IDEContext)
	if strings.TrimSpace(ideContext) == "" {
		return contexts
	}
	extra := book.FormatCompactContextParts([]book.CompactContextPart{{
		ID:          "ide_current_state",
		Source:      "frontend:ide_context",
		Title:       "IDE 当前状态",
		PromptTitle: fmt.Sprintf("IDE 当前状态（前端请求提供，仅路径；最多 %d 个打开文件）", ideContextMaxOpenFiles),
		Content:     ideContext,
	}})
	contexts.Dynamic = strings.TrimSpace(strings.Join(trimmedNonEmpty([]string{contexts.Dynamic, extra}), "\n\n"))
	return contexts
}

func IDEContextRuntimeContext(ide IDEContextRef) string {
	currentFile := boundedIDEContextPath(ide.CurrentFile)
	openFiles := boundedIDEContextPaths(ide.OpenFiles, ideContextMaxOpenFiles)
	if currentFile == "" && len(openFiles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("来源：前端 IDE 请求状态；仅描述当前打开/聚焦的文件路径，不包含文件正文。\n")
	if currentFile != "" {
		sb.WriteString("- 当前聚焦文件：")
		sb.WriteString(currentFile)
		sb.WriteString("\n")
	} else {
		sb.WriteString("- 当前聚焦文件：无\n")
	}
	if len(openFiles) > 0 {
		sb.WriteString("- 当前打开文件：")
		sb.WriteString(strings.Join(openFiles, "、"))
		if len(ide.OpenFiles) > len(openFiles) {
			sb.WriteString(fmt.Sprintf("（其余 %d 个已省略）", len(ide.OpenFiles)-len(openFiles)))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("- 使用约束：如需读取正文，必须按路径显式使用工具读取；不要假设这里包含最新文件内容。")
	return strings.TrimSpace(sb.String())
}

func boundedIDEContextPaths(paths []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	result := make([]string, 0, min(len(paths), limit))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = boundedIDEContextPath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func boundedIDEContextPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimLeft(path, "/")
	if path == "" || strings.Contains(path, ":") {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return ""
		}
	}
	runes := []rune(path)
	if len(runes) <= ideContextMaxPathRunes {
		return path
	}
	return string(runes[:ideContextMaxPathRunes]) + "[已截断]"
}

func IDEWorkspaceRuntimeContext(state *book.State) string {
	if state == nil {
		return ""
	}
	contexts := IDEWorkspaceRuntimeContextsForState(state)
	parts := make([]book.CompactContextPart, 0, 2)
	if contexts.Stable != "" {
		parts = append(parts, book.CompactContextPart{PromptTitle: contexts.StableTitle, Content: contexts.Stable})
	}
	if contexts.Dynamic != "" {
		parts = append(parts, book.CompactContextPart{PromptTitle: contexts.DynamicTitle, Content: contexts.Dynamic})
	}
	if context := strings.TrimSpace(book.FormatCompactContextParts(parts)); context != "" {
		return context
	}
	return prompts.EmptyIDEStateHint()
}

func BuildInteractiveStoryInstruction(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) string {
	return BuildInteractiveStoryInstructionComposition(cfg, state, teller).Instruction()
}

func buildInteractiveStoryBuiltinInstruction(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (string, string, string) {
	workspace := ""
	replyTargetChars := 0
	if cfg != nil {
		workspace = cfg.Workspace
		replyTargetChars = cfg.InteractiveReplyTargetChars
	}
	creator := ""
	if state != nil {
		creator = state.ReadCreatorPrompt()
	}
	builtIn := prompts.BuildInteractiveStorySystemInstruction(prompts.InteractiveStorySystemInstructionInput{
		CreatorPrompt:           creator,
		Workspace:               workspace,
		ReplyTargetChars:        replyTargetChars,
		StoryTellerID:           teller.StoryTellerID,
		StoryTellerName:         teller.StoryTellerName,
		StoryTellerDescription:  teller.StoryTellerDescription,
		StoryTellerSystemPrompt: teller.StoryTellerSystemPrompt,
		StyleRules:              boundedStyleRules(teller.StyleRules, maxStyleRuleContextChars),
	})
	return builtIn, workspace, creator
}

func buildImageBuiltinInstruction(cfg *config.Config, state *book.State, systemPrompt string) (string, string, string) {
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
	parts := []string{
		"你是 Nova 的通用图像 Agent，负责把调用方提供的有界上下文转换成图像生成请求。",
		"必须先理解本次 purpose、source_context、调用方系统提示和已加载 Skill，再调用 generate_image 工具生成图像。",
		"只能生成图像和图像元数据，不得修改故事正文、章节正文、资料库、配置或其他 workspace 内容。",
		"图像提示词应清晰描述主体、场景、构图、光线、视觉风格、情绪和需要避免的文字、水印、logo。",
		"如果调用方要求加载 Skill，必须先用 skill 工具读取完整 Skill 后再调用 generate_image。",
	}
	if strings.TrimSpace(creator) != "" {
		parts = append(parts, "可参考 CREATOR.md 中稳定的作品基调，但不得复制大段原文。")
	}
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		parts = append(parts, "## 调用点系统提示\n\n"+trimmed)
	}
	return strings.Join(parts, "\n\n"), workspace, creator
}

// BuiltinAgentPrompts returns the default system prompts shown in the Agents
// settings page. The result is read-only display data; persisted overrides
// still live under config.Settings.AgentPrompts.
func BuiltinAgentPrompts(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	return config.AgentPromptSettings{
		IDE:                   config.AgentPromptOverride{SystemPrompt: BuildInstruction(promptCfg, state, ideTeller)},
		InteractiveStory:      config.AgentPromptOverride{SystemPrompt: BuildInteractiveStoryInstruction(promptCfg, state, prompts.InteractiveStorySystemInstructionInput{})},
		ConfigManager:         config.AgentPromptOverride{SystemPrompt: BuildConfigManagerInstruction(promptCfg, state)},
		InteractiveState:      config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction())},
		InteractiveHotChoices: config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction())},
		VersionSummary:        config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。")},
		ToolAgent:             config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindToolAgent, chapterSplitRegexSystemInstruction())},
		Image:                 config.AgentPromptOverride{SystemPrompt: BuildImageInstruction(promptCfg, state, "")},
		Automation:            config.AgentPromptOverride{SystemPrompt: BuildAutomationInstruction(promptCfg, state, AutomationTaskInstruction{})},
		ContextCompaction:     config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindContextCompaction, contextCompactionSystemInstruction())},
	}
}

func BuiltinAgentPromptBlocks(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptBlockSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	_, ideWorkspace, _, _ := buildIDEBuiltinInstruction(promptCfg, state, ideTeller)
	_, interactiveWorkspace, _ := buildInteractiveStoryBuiltinInstruction(promptCfg, state, prompts.InteractiveStorySystemInstructionInput{})
	configManagerFlow := configManagerFlowInstruction(promptCfg, state)
	return config.AgentPromptBlockSettings{
		IDE:                   builtinPromptBlocks(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace)),
		InteractiveStory:      builtinPromptBlocks(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		ConfigManager:         builtinPromptBlocks(promptCfg, config.AgentKindConfigManager, configManagerFlow),
		InteractiveState:      builtinPromptBlocks(promptCfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptBlocks(promptCfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptBlocks(promptCfg, config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptBlocks(promptCfg, config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Image:                 builtinPromptBlocks(promptCfg, config.AgentKindImage, ""),
		Automation:            builtinPromptBlocks(promptCfg, config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
		ContextCompaction:     builtinPromptBlocks(promptCfg, config.AgentKindContextCompaction, contextCompactionSystemInstruction()),
	}
}

func BuiltinAgentPromptSources(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptSourceSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	_, ideWorkspace, ideCreator, _ := buildIDEBuiltinInstruction(promptCfg, state, ideTeller)
	_, interactiveWorkspace, interactiveCreator := buildInteractiveStoryBuiltinInstruction(promptCfg, state, prompts.InteractiveStorySystemInstructionInput{})
	configManagerFlow := configManagerFlowInstruction(promptCfg, state)
	configManagerCreator := ""
	if state != nil {
		configManagerCreator = state.ReadCreatorPrompt()
	}
	return config.AgentPromptSourceSettings{
		IDE: builtinPromptSourceList(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", ideCreator),
			readonlyPromptSource("teller", "IDE 默认导演规则", ideTeller.ID, ideTeller.Prompt),
		),
		InteractiveStory: builtinPromptSourceList(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", interactiveCreator),
		),
		ConfigManager:         builtinPromptSourceList(promptCfg, config.AgentKindConfigManager, configManagerFlow, readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", configManagerCreator)),
		InteractiveState:      builtinPromptSourceList(promptCfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptSourceList(promptCfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptSourceList(promptCfg, config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptSourceList(promptCfg, config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Image:                 builtinPromptSourceList(promptCfg, config.AgentKindImage, ""),
		Automation:            builtinPromptSourceList(promptCfg, config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
		ContextCompaction:     builtinPromptSourceList(promptCfg, config.AgentKindContextCompaction, contextCompactionSystemInstruction()),
	}
}

func builtinPromptBlocks(cfg *config.Config, agentKind, flow string) config.AgentPromptBlocks {
	return config.AgentPromptBlocks{
		RuntimeContract:      runtimeContractForAgent(cfg, agentKind),
		OutputProtocol:       outputProtocolForAgent(agentKind),
		EditableSystemPrompt: editablePromptFlowForAgent(agentKind, flow),
	}
}

func builtinPromptSourceList(cfg *config.Config, agentKind, flow string, extraSources ...config.AgentPromptSource) config.AgentPromptSourceList {
	sources := make([]config.AgentPromptSource, 0, len(extraSources)+4)
	sources = append(sources, config.AgentPromptSource{
		ID:      "runtime_contract",
		Title:   "运行契约",
		Source:  "Nova runtime",
		Content: runtimeContractForAgent(cfg, agentKind),
	})
	if outputProtocol := strings.TrimSpace(outputProtocolForAgent(agentKind)); outputProtocol != "" {
		sources = append(sources, config.AgentPromptSource{
			ID:      "output_protocol",
			Title:   "输出格式",
			Source:  "Nova runtime",
			Content: outputProtocol,
		})
	}
	for _, source := range extraSources {
		if strings.TrimSpace(source.Content) != "" {
			sources = append(sources, source)
		}
	}
	sources = append(sources, config.AgentPromptSource{
		ID:       "flow",
		Title:    "流程规则",
		Source:   "Nova built-in",
		Content:  editablePromptFlowForAgent(agentKind, flow),
		Editable: true,
		Field:    "flow_prompt",
	})
	sources = append(sources, config.AgentPromptSource{
		ID:       "custom",
		Title:    "用户自定义",
		Source:   "user/workspace config",
		Editable: true,
		Field:    "system_prompt",
	})
	return config.AgentPromptSourceList{Sources: sources}
}

func readonlyPromptSource(id, title, source, content string) config.AgentPromptSource {
	return config.AgentPromptSource{
		ID:      id,
		Title:   title,
		Source:  source,
		Content: strings.TrimSpace(content),
	}
}

func styleRulePromptSources(rules []StyleRule) []promptSource {
	sources := make([]promptSource, 0, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.StyleContents) == 0 {
			continue
		}
		content := styleRulesSystemInstruction([]StyleRule{rule})
		if strings.TrimSpace(content) == "" {
			continue
		}
		sources = append(sources, promptSource{
			source:  "系统提示",
			title:   "场景化风格规则：" + scene,
			content: content,
			note:    "当前叙事方案",
		})
	}
	return sources
}

func ideFlowInstruction(cfg *config.Config, workspace string) string {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return prompts.BuildIDEWritingFlowInstruction(prompts.SystemInstructionInput{
		Workspace:             workspace,
		ChapterFilenameFormat: cfg.ChapterFilenameFormat,
		VolumeDirFormat:       cfg.VolumeDirFormat,
		ChapterGroupMin:       cfg.ChapterGroupMin,
		ChapterGroupMax:       cfg.ChapterGroupMax,
	})
}

func interactiveStoryFlowInstruction(cfg *config.Config, workspace string) string {
	return prompts.BuildInteractiveStoryFlowInstruction(prompts.InteractiveStorySystemInstructionInput{
		Workspace: workspace,
	})
}

func editablePromptFlowForAgent(agentKind, flow string) string {
	switch agentKind {
	case config.AgentKindInteractiveState:
		return ""
	case config.AgentKindInteractiveHotChoices:
		return filterPromptLines(flow, "必须只输出", "不要输出")
	case config.AgentKindVersionSummary:
		return ""
	case config.AgentKindToolAgent:
		return filterPromptLines(flow, "只输出 JSON", "不要返回 Markdown")
	default:
		return strings.TrimSpace(flow)
	}
}

func BuildConfigManagerInstruction(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) string {
	return BuildConfigManagerInstructionComposition(cfg, state, resourceSkills...).Instruction()
}

func appendConfigManagerResourceSkills(builtIn string, resourceSkills []ConfigManagerResourceSkill) string {
	var sb strings.Builder
	for _, skill := range resourceSkills {
		name := strings.TrimSpace(skill.Name)
		content := strings.TrimSpace(skill.Content)
		if name == "" || content == "" {
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString("\n\n## 本轮自动加载的配置 Skills\n\n")
			sb.WriteString("以下内容来自当前生效的 Nova Skills，用于在调用复杂 write_* 配置工具前确认 JSON 结构、枚举、默认值和安全流程；若与运行时契约或后端校验冲突，以运行时契约和后端校验为准。\n")
		}
		sb.WriteString("\n### /")
		sb.WriteString(name)
		sb.WriteString("\n\n")
		if description := strings.TrimSpace(skill.Description); description != "" {
			sb.WriteString("description: ")
			sb.WriteString(description)
			sb.WriteString("\n\n")
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return builtIn
	}
	return strings.TrimSpace(builtIn) + sb.String()
}

func buildConfigManagerBuiltinInstruction(cfg *config.Config, state *book.State) (string, string, string) {
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
	return configManagerFlowInstructionFor(workspace, creator), workspace, creator
}

func configManagerFlowInstruction(cfg *config.Config, state *book.State) string {
	builtIn, _, _ := buildConfigManagerBuiltinInstruction(cfg, state)
	return builtIn
}

func configManagerFlowInstructionFor(workspace, creator string) string {
	var sb strings.Builder
	sb.WriteString("你是 Nova 的统一配置管理 Agent，负责在模块内嵌入口中帮助用户管理资料库、方案预设（叙事方案和图像方案）、自动化任务、Skills、故事记忆结构和故事记忆记录。\n\n")
	if strings.TrimSpace(workspace) != "" {
		sb.WriteString("当前作品 workspace: ")
		sb.WriteString(strings.TrimSpace(workspace))
		sb.WriteString("\n\n")
	}
	if strings.TrimSpace(creator) != "" {
		sb.WriteString("## CREATOR.md 创作约束\n\n")
		sb.WriteString(strings.TrimSpace(creator))
		sb.WriteString("\n\n")
	}
	sb.WriteString(strings.Join([]string{
		"## 工作方式",
		"- 根据用户所在模块和当前资源上下文，优先使用对应模块工具完成管理任务。",
		"- 每个模块先用 list 工具查看索引；需要详情时再用 read 工具批量读取。故事记忆结构没有 read 工具，list 已返回完整结构。",
		"- 增删改统一使用对应 write 工具批量完成，写入后用简短中文总结实际变更。",
		"- Agent 页配置使用 list_agent_configs 一次读取全量配置，再用 write_agent_configs 写入；写入必须显式指定 scope=user 或 scope=workspace。",
		"- 不要修改端口、主题、远程访问、编辑器外观等非 Agent 页设置。",
		"- 不要通过文件工具直接改资料库、方案预设、自动化、Skills、故事记忆或 Agent 配置的底层存储文件。",
		"- 删除、隐藏、覆盖、大范围重写必须有用户明确指令；缺少明确指令时先询问。",
		"",
		"## 模块边界",
		"- 资料库记录长期稳定设定；短期位置、伤势、心理、目标优先进入故事记忆或写作状态，不默认写资料库。",
		"- 叙事方案只维护导演/讲述规则、槽位和互动生成偏好，不写故事正文；图像方案只维护视觉风格、媒介、构图、限制和避免项。",
		"- Skills 写入 SKILL.md 文档，必须说明适用场景、上下文获取和具体工作流；内置预制 Skill 只能通过工作区同名覆盖修改，不得写入内置 Skills 目录。",
		"- 自动化任务必须保持触发条件、通知/执行策略和写入权限清晰。",
		"- 故事记忆结构定义字段和生成规则；故事记忆记录保存具体故事状态，两者必须分开操作。",
	}, "\n"))
	return sb.String()
}

type promptSource struct {
	source  string
	title   string
	content string
	note    string
}

func systemPromptSourceSummary(mode, creator string, stateParts []book.CompactContextPart, extraSources ...promptSource) string {
	contextLog := newContextBuildLog()
	if strings.TrimSpace(creator) != "" {
		contextLog.add("系统提示", "CREATOR.md", creator, "创作者指令")
	}
	for _, source := range extraSources {
		if strings.TrimSpace(source.content) == "" {
			continue
		}
		contextLog.add(source.source, source.title, source.content, source.note)
	}
	for _, section := range stateParts {
		if strings.TrimSpace(section.Content) == "" {
			continue
		}
		contextLog.add("作品状态", section.Title, section.Content, section.Source)
	}
	contextLog.add("系统提示", "Nova "+mode+" 内置规则", "基础规则、工具边界、工作流约束", "")
	return contextLog.String()
}

func promptPartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + boolString(s != ""),
		"bytes=" + intString(len(s)),
		"chars=" + intString(utf8.RuneCountInString(s)),
		"lines=" + intString(promptLineCount(s)),
		"sha=" + shortSHA256(s),
		"preview=" + strconv.Quote(safeLogPreview(s, 80)),
	}, ",")
}

func filterPromptLines(content string, blockedPrefixes ...string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		blocked := false
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intString(v int) string {
	return strconv.Itoa(v)
}

func promptLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func shortSHA256(s string) string {
	if s == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
