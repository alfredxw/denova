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
	builtIn, workspace, creator, stateContext := buildIDEBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindIDE, builtIn)
	logSystemPromptComposition("ide", workspace, creator, stateContext, instruction, promptSource{
		source:  "系统提示",
		title:   "写作模式默认导演规则",
		content: teller.Prompt,
		note:    teller.ID,
	})
	return instruction
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
	builtIn, workspace, creator := buildInteractiveStoryBuiltinInstruction(cfg, state, teller)
	instruction := protectedSystemInstruction(cfg, config.AgentKindInteractiveStory, builtIn)
	logSystemPromptComposition("interactive", workspace, creator, "", instruction, promptSource{
		source:  "系统提示",
		title:   "导演系统规则",
		content: teller.StoryTellerSystemPrompt,
		note:    teller.StoryTellerID,
	})
	return instruction
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
		LoreEditor:            config.AgentPromptOverride{SystemPrompt: BuildLoreAgentInstruction(promptCfg, state)},
		TellerEditor:          config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindTellerEditor, tellerEditorSystemInstruction())},
		InteractiveState:      config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction())},
		InteractiveHotChoices: config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction())},
		VersionSummary:        config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。")},
		ToolAgent:             config.AgentPromptOverride{SystemPrompt: protectedSystemInstruction(promptCfg, config.AgentKindToolAgent, chapterSplitRegexSystemInstruction())},
		Automation:            config.AgentPromptOverride{SystemPrompt: BuildAutomationInstruction(promptCfg, state, AutomationTaskInstruction{})},
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
	_, loreWorkspace, _ := buildLoreBuiltinInstruction(promptCfg, state)
	return config.AgentPromptBlockSettings{
		IDE:                   builtinPromptBlocks(config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace)),
		InteractiveStory:      builtinPromptBlocks(config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		LoreEditor:            builtinPromptBlocks(config.AgentKindLoreEditor, loreFlowInstruction(loreWorkspace)),
		TellerEditor:          builtinPromptBlocks(config.AgentKindTellerEditor, tellerEditorSystemInstruction()),
		InteractiveState:      builtinPromptBlocks(config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptBlocks(config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptBlocks(config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptBlocks(config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Automation:            builtinPromptBlocks(config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
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
	_, loreWorkspace, loreCreator := buildLoreBuiltinInstruction(promptCfg, state)
	return config.AgentPromptSourceSettings{
		IDE: builtinPromptSourceList(config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", ideCreator),
			readonlyPromptSource("teller", "IDE 默认导演规则", ideTeller.ID, ideTeller.Prompt),
			readonlyPromptSource("workspace_context", "当前作品状态", "workspace state", ideStateContext),
		),
		InteractiveStory: builtinPromptSourceList(config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace),
			readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", interactiveCreator),
		),
		LoreEditor:            builtinPromptSourceList(config.AgentKindLoreEditor, loreFlowInstruction(loreWorkspace), readonlyPromptSource("creator", "CREATOR.md", "CREATOR.md", loreCreator)),
		TellerEditor:          builtinPromptSourceList(config.AgentKindTellerEditor, tellerEditorSystemInstruction()),
		InteractiveState:      builtinPromptSourceList(config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction()),
		InteractiveHotChoices: builtinPromptSourceList(config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction()),
		VersionSummary:        builtinPromptSourceList(config.AgentKindVersionSummary, "你是 Nova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"),
		ToolAgent:             builtinPromptSourceList(config.AgentKindToolAgent, chapterSplitRegexSystemInstruction()),
		Automation:            builtinPromptSourceList(config.AgentKindAutomation, editableAutomationBuiltinInstruction(promptCfg, state, AutomationTaskInstruction{})),
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
	replyTargetChars := 0
	if cfg != nil {
		replyTargetChars = cfg.InteractiveReplyTargetChars
	}
	return prompts.BuildInteractiveStoryFlowInstruction(prompts.InteractiveStorySystemInstructionInput{
		Workspace:        workspace,
		ReplyTargetChars: replyTargetChars,
	})
}

func loreFlowInstruction(workspace string) string {
	return prompts.BuildLoreAgentFlowInstruction(prompts.LoreAgentSystemInstructionInput{Workspace: workspace})
}

func editablePromptFlowForAgent(agentKind, flow string) string {
	switch agentKind {
	case config.AgentKindTellerEditor:
		_, rules, ok := strings.Cut(flow, "规则：")
		if ok {
			return "规则：\n" + strings.TrimSpace(rules)
		}
		return ""
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

func BuildLoreAgentInstruction(cfg *config.Config, state *book.State) string {
	builtIn, workspace, creator := buildLoreBuiltinInstruction(cfg, state)
	instruction := protectedSystemInstruction(cfg, config.AgentKindLoreEditor, builtIn)
	logSystemPromptComposition("lore", workspace, creator, "", instruction, promptSource{
		source:  "系统提示",
		title:   "资料库 Agent 内置规则",
		content: builtIn,
		note:    "tool-chain",
	})
	return instruction
}

func buildLoreBuiltinInstruction(cfg *config.Config, state *book.State) (string, string, string) {
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
	builtIn := prompts.BuildLoreAgentSystemInstruction(prompts.LoreAgentSystemInstructionInput{
		CreatorPrompt: creator,
		Workspace:     workspace,
	})
	return builtIn, workspace, creator
}

type promptSource struct {
	source  string
	title   string
	content string
	note    string
}

func logSystemPromptComposition(mode, workspace, creator, stateContext, instruction string, extraSources ...promptSource) {
	log.Printf(
		"[agent-prompt] system composition mode=%s workspace=%s creator=%s state=%s instruction=%s",
		mode,
		workspace,
		promptPartSummary(creator),
		promptPartSummary(stateContext),
		promptPartSummary(instruction),
	)
	log.Printf("[agent-prompt] system sources mode=%s workspace=%s sources=%s", mode, workspace, systemPromptSourceSummary(mode, creator, stateContext, extraSources...))
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
