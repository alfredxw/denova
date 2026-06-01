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

// IDEStoryTeller 描述 IDE 创作 Agent 本轮使用的默认讲述者规则。
type IDEStoryTeller struct {
	ID          string
	Name        string
	Description string
	Prompt      string
}

// BuildInstruction 构建系统指令，包含基础 prompt + 作品状态注入。
// 实际的 Prompt 文本集中在 internal/prompts 包，这里只负责把 cfg/state 翻译成 prompts.SystemInstructionInput。
func BuildInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) string {
	creator := state.ReadCreatorPrompt()
	stateContext := state.CompactContext()
	instruction := prompts.BuildSystemInstruction(prompts.SystemInstructionInput{
		CreatorPrompt:          creator,
		Workspace:              cfg.Workspace,
		StateContext:           stateContext,
		StoryTellerID:          teller.ID,
		StoryTellerName:        teller.Name,
		StoryTellerDescription: teller.Description,
		StoryTellerPrompt:      teller.Prompt,
		ChapterFilenameFormat:  cfg.ChapterFilenameFormat,
		DraftFlowEnabled:       cfg.DraftFlowEnabled,
		ChapterGroupMin:        cfg.ChapterGroupMin,
		ChapterGroupMax:        cfg.ChapterGroupMax,
	})
	logSystemPromptComposition("ide", cfg.Workspace, creator, stateContext, instruction, promptSource{
		source:  "系统提示",
		title:   "IDE 默认讲述者规则",
		content: teller.Prompt,
		note:    teller.ID,
	})
	return instruction
}

func BuildInteractiveStoryInstruction(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) string {
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
	instruction := prompts.BuildInteractiveStorySystemInstruction(prompts.InteractiveStorySystemInstructionInput{
		CreatorPrompt:           creator,
		Workspace:               workspace,
		ReplyTargetChars:        replyTargetChars,
		StoryTellerID:           teller.StoryTellerID,
		StoryTellerName:         teller.StoryTellerName,
		StoryTellerDescription:  teller.StoryTellerDescription,
		StoryTellerSystemPrompt: teller.StoryTellerSystemPrompt,
	})
	logSystemPromptComposition("interactive", workspace, creator, "", instruction, promptSource{
		source:  "系统提示",
		title:   "讲述者系统规则",
		content: teller.StoryTellerSystemPrompt,
		note:    teller.StoryTellerID,
	})
	return instruction
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
	case "章节组细纲":
		return "setting/chapter-groups/"
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
