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

// BuildInstruction 构建系统指令，包含基础 prompt + 作品状态注入。
// 实际的 Prompt 文本集中在 internal/prompts 包，这里只负责把 cfg/state 翻译成 prompts.SystemInstructionInput。
func BuildInstruction(cfg *config.Config, state *book.State) string {
	creator := state.ReadCreatorPrompt()
	stateContext := state.CompactContext()
	instruction := prompts.BuildSystemInstruction(prompts.SystemInstructionInput{
		CreatorPrompt: creator,
		Workspace:     cfg.Workspace,
		StateContext:  stateContext,
	})
	logSystemPromptComposition("ide", cfg.Workspace, creator, stateContext, instruction)
	return instruction
}

func BuildInteractiveStoryInstruction(cfg *config.Config, state *book.State) string {
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
		CreatorPrompt:    creator,
		Workspace:        workspace,
		ReplyTargetChars: replyTargetChars,
	})
	logSystemPromptComposition("interactive", workspace, creator, "", instruction)
	return instruction
}

func logSystemPromptComposition(mode, workspace, creator, stateContext, instruction string) {
	log.Printf(
		"[agent-prompt] system composition mode=%s workspace=%s creator=%s state=%s instruction=%s",
		mode,
		workspace,
		promptPartSummary(creator),
		promptPartSummary(stateContext),
		promptPartSummary(instruction),
	)
}

func promptPartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + boolString(s != ""),
		"bytes=" + intString(len(s)),
		"chars=" + intString(utf8.RuneCountInString(s)),
		"lines=" + intString(promptLineCount(s)),
		"sha=" + shortSHA256(s),
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
