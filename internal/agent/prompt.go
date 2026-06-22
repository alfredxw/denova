package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"unicode/utf8"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
)

// IDEStoryTeller 描述写作 Agent 本轮使用的默认导演规则。
type IDEStoryTeller struct {
	ID          string
	Name        string
	Description string
	Prompt      string
}

// BuildInstruction 构建系统指令，包含基础 prompt + 作品状态注入。
// 实际的 Prompt 文本集中在 internal/prompts 包，这里只负责把 cfg/state 翻译成 prompts.SystemInstructionInput。
func BuildInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) string {
	return BuildInstructionComposition(cfg, state, teller).Instruction()
}

// BuildInstructionComposition returns the IDE system prompt and its auditable source summary.
func BuildInstructionComposition(cfg *config.Config, state *book.State, teller IDEStoryTeller) SystemPromptCompositionLog {
	builtIn, workspace, creator, stateContext := buildIDEBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindIDE, builtIn)
	return SystemPromptCompositionLog{
		mode:         "ide",
		workspace:    workspace,
		creator:      creator,
		stateContext: stateContext,
		instruction:  instruction,
		extraSources: []promptSource{{
			source:  "系统提示",
			title:   "写作模式默认导演规则",
			content: teller.Prompt,
			note:    teller.ID,
		}},
	}
}

// SystemPromptCompositionLog records the source breakdown for one system prompt.
type SystemPromptCompositionLog struct {
	mode         string
	workspace    string
	creator      string
	stateContext string
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
	log.Printf("[agent-prompt] system sources mode=%s workspace=%s task_id=%s session_id=%s sources=%s", l.mode, l.workspace, options.TaskID, options.SessionID, systemPromptSourceSummary(l.mode, l.creator, l.stateContext, l.extraSources...))
}

func newInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) SystemPromptCompositionLog {
	builtIn, workspace, creator := buildInteractiveStoryBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindInteractiveStory, builtIn)
	return SystemPromptCompositionLog{
		mode:        "interactive",
		workspace:   workspace,
		creator:     creator,
		instruction: instruction,
		extraSources: []promptSource{{
			source:  "系统提示",
			title:   "导演系统规则",
			content: teller.StoryTellerSystemPrompt,
			note:    teller.StoryTellerID,
		}},
	}
}

// BuildInteractiveStoryInstructionComposition returns the interactive story prompt and its source summary.
func BuildInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) SystemPromptCompositionLog {
	return newInteractiveStoryInstructionComposition(cfg, state, teller)
}

// BuildConfigManagerInstructionComposition returns the config manager prompt and its source summary.
func BuildConfigManagerInstructionComposition(cfg *config.Config, state *book.State) SystemPromptCompositionLog {
	builtIn, workspace, creator := buildConfigManagerBuiltinInstruction(cfg, state)
	instruction := protectedSystemInstruction(cfg, config.AgentKindConfigManager, builtIn)
	return SystemPromptCompositionLog{
		mode:        "config_manager",
		workspace:   workspace,
		creator:     creator,
		instruction: instruction,
		extraSources: []promptSource{{
			source:  "系统提示",
			title:   "配置管理 Agent 内置规则",
			content: builtIn,
			note:    "tool-chain",
		}},
	}
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
		ChapterFilenameFormat:  cfg.ChapterFilenameFormat,
		VolumeDirFormat:        cfg.VolumeDirFormat,
		DraftFlowEnabled:       cfg.DraftFlowEnabled,
		ChapterGroupMin:        cfg.ChapterGroupMin,
		ChapterGroupMax:        cfg.ChapterGroupMax,
	})
	return builtIn, workspace, creator, stateContext
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
	})
	return builtIn, workspace, creator
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
		IDE:                   builtinPromptBlocks(config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace)),
		InteractiveStory:      builtinPromptBlocks(config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		ConfigManager:         builtinPromptBlocks(config.AgentKindConfigManager, configManagerFlow),
		InteractiveState:      builtinPromptBlocks(config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptBlocks(config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptBlocks(config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptBlocks(config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Automation:            builtinPromptBlocks(config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
		ContextCompaction:     builtinPromptBlocks(config.AgentKindContextCompaction, contextCompactionSystemInstruction()),
	}
}

func BuiltinAgentPromptSources(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptSourceSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	_, ideWorkspace, ideCreator, ideStateContext := buildIDEBuiltinInstruction(promptCfg, state, ideTeller)
	_, interactiveWorkspace, interactiveCreator := buildInteractiveStoryBuiltinInstruction(promptCfg, state, prompts.InteractiveStorySystemInstructionInput{})
	configManagerFlow := configManagerFlowInstruction(promptCfg, state)
	configManagerCreator := ""
	if state != nil {
		configManagerCreator = state.ReadCreatorPrompt()
	}
	return config.AgentPromptSourceSettings{
		IDE: builtinPromptSourceList(config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", ideCreator),
			readonlyPromptSource("teller", "IDE 默认导演规则", ideTeller.ID, ideTeller.Prompt),
			readonlyPromptSource("workspace_context", "当前作品状态", "workspace state", ideStateContext),
		),
		InteractiveStory: builtinPromptSourceList(config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", interactiveCreator),
		),
		ConfigManager:         builtinPromptSourceList(config.AgentKindConfigManager, configManagerFlow, readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", configManagerCreator)),
		InteractiveState:      builtinPromptSourceList(config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptSourceList(config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptSourceList(config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptSourceList(config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Automation:            builtinPromptSourceList(config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
		ContextCompaction:     builtinPromptSourceList(config.AgentKindContextCompaction, contextCompactionSystemInstruction()),
	}
}

func builtinPromptBlocks(agentKind, flow string) config.AgentPromptBlocks {
	return config.AgentPromptBlocks{
		RuntimeContract:      runtimeContractForAgent(agentKind),
		OutputProtocol:       outputProtocolForAgent(agentKind),
		EditableSystemPrompt: editablePromptFlowForAgent(agentKind, flow),
	}
}

func builtinPromptSourceList(agentKind, flow string, extraSources ...config.AgentPromptSource) config.AgentPromptSourceList {
	sources := make([]config.AgentPromptSource, 0, len(extraSources)+4)
	sources = append(sources, config.AgentPromptSource{
		ID:      "runtime_contract",
		Title:   "运行契约",
		Source:  "Nova runtime",
		Content: runtimeContractForAgent(agentKind),
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

func ideFlowInstruction(cfg *config.Config, workspace string) string {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return prompts.BuildIDEWritingFlowInstruction(prompts.SystemInstructionInput{
		Workspace:             workspace,
		ChapterFilenameFormat: cfg.ChapterFilenameFormat,
		VolumeDirFormat:       cfg.VolumeDirFormat,
		DraftFlowEnabled:      cfg.DraftFlowEnabled,
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

func BuildConfigManagerInstruction(cfg *config.Config, state *book.State) string {
	return BuildConfigManagerInstructionComposition(cfg, state).Instruction()
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
	sb.WriteString("你是 Nova 的统一配置管理 Agent，负责在模块内嵌入口中帮助用户管理资料库、叙事编排、自动化任务、Skills、故事记忆结构和故事记忆记录。\n\n")
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
		"- 不要修改 Nova 设置、模型、端口、主题、Agent prompt 或工具权限。",
		"- 不要通过文件工具直接改资料库、导演、自动化、Skills 或故事记忆的底层存储文件。",
		"- 删除、隐藏、覆盖、大范围重写必须有用户明确指令；缺少明确指令时先询问。",
		"",
		"## 模块边界",
		"- 资料库记录长期稳定设定；短期位置、伤势、心理、目标优先进入故事记忆或写作状态，不默认写资料库。",
		"- 叙事编排只维护导演/讲述规则、槽位和互动生成偏好，不写故事正文。",
		"- Skills 写入 SKILL.md 文档，必须说明适用场景、上下文获取和具体工作流。",
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

func systemPromptSourceSummary(mode, creator, stateContext string, extraSources ...promptSource) string {
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
	for _, section := range promptStateSections(stateContext) {
		contextLog.add("作品状态", section.Title, section.Content, section.Source)
	}
	contextLog.add("系统提示", "Nova "+mode+" 内置规则", "基础规则、工具边界、工作流约束", "")
	return contextLog.String()
}

type promptStateSection struct {
	Title   string
	Source  string
	Content string
}

func promptStateSections(stateContext string) []promptStateSection {
	stateContext = strings.TrimSpace(stateContext)
	if stateContext == "" {
		return nil
	}
	blocks := strings.Split("\n"+stateContext, "\n## ")
	sections := make([]promptStateSection, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		title, content, _ := strings.Cut(block, "\n")
		title = strings.TrimSpace(title)
		content = strings.TrimSpace(content)
		if title == "" || content == "" {
			continue
		}
		sections = append(sections, promptStateSection{
			Title:   title,
			Source:  promptStateSectionSource(title),
			Content: content,
		})
	}
	return sections
}

func promptStateSectionSource(title string) string {
	switch title {
	case "当前大纲":
		return "setting/outline.md"
	case "当前进度":
		return "setting/progress.md"
	case "角色状态":
		return "setting/character-states.md"
	case "章节组细纲":
		return "setting/chapter-groups/"
	case "章节目录概览":
		return "chapters/"
	case "资料库":
		return ".nova/lore/items.json"
	default:
		return "作品状态注入"
	}
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
