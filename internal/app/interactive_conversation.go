package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

type interactiveConversation struct {
	store            *interactive.Store
	novaDir          string
	workspace        string
	storyID          string
	branchID         string
	user             string
	replyTargetChars int
}

func newInteractiveConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int) *interactiveConversation {
	return &interactiveConversation{store: store, novaDir: novaDir, workspace: workspace, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
}

func (c *interactiveConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	_ = originalMessage
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	tellerPrompt := c.tellerPrompt(storyCtx.Meta.StoryTellerID)
	stateJSON, err := json.MarshalIndent(storyCtx.Snapshot.State, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化互动状态失败: %w", err)
	}
	characters := c.readSettingFile("characters.md")
	worldBuilding := c.readSettingFile("world-building.md")
	contextMessage := prompts.InteractiveStoryContext(prompts.InteractiveStoryPromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		StoryTeller:       tellerPrompt,
		BranchID:          storyCtx.Snapshot.BranchID,
		ReplyTargetChars:  c.replyTargetChars,
		Characters:        characters,
		WorldBuilding:     worldBuilding,
		SnapshotStateJSON: string(stateJSON),
	})
	history := make([]*schema.Message, 0, len(storyCtx.Snapshot.Turns)*2+2)
	history = append(history, schema.UserMessage(contextMessage))
	for _, turn := range storyCtx.Snapshot.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage)))
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s teller_prompt=%s characters=%s world_building=%s snapshot_state=%s turns=%d history=%s turn_instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		interactivePartSummary(tellerPrompt),
		interactivePartSummary(characters),
		interactivePartSummary(worldBuilding),
		interactivePartSummary(string(stateJSON)),
		len(storyCtx.Snapshot.Turns),
		interactiveMessageListSummary(history),
		interactivePartSummary(history[len(history)-1].Content),
	)
	return history, nil
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *interactiveConversation) AppendAssistantWithThinking(content, thinking string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	log.Printf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content)
	narrative, ops, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q ops=%s", c.storyID, c.branchID, narrative, interactiveStateOpsLogJSON(ops))
	_, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:  c.branchID,
		User:      c.user,
		Narrative: narrative,
		Thinking:  thinking,
		Ops:       ops,
	})
	return err
}

func (c *interactiveConversation) tellerPrompt(tellerID string) string {
	if c.novaDir == "" {
		return ""
	}
	teller, err := interactive.NewTellerLibrary(c.novaDir).Get(tellerID)
	if err == nil {
		return teller.Prompt
	}
	log.Printf("[interactive-agent] load teller failed id=%s err=%v", tellerID, err)
	fallback, fallbackErr := interactive.NewTellerLibrary(c.novaDir).Get("classic")
	if fallbackErr != nil {
		log.Printf("[interactive-agent] load fallback teller failed err=%v", fallbackErr)
		return ""
	}
	return fallback.Prompt
}

func (c *interactiveConversation) readSettingFile(name string) string {
	if c.workspace == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(c.workspace, "setting", name))
	if err != nil {
		return ""
	}
	return string(data)
}

func (c *interactiveConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	log.Printf("[interactive-agent] interruption ignored story_id=%s branch_id=%s reason=%s", c.storyID, c.branchID, reason)
	return nil
}

func (c *interactiveConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *interactiveConversation) ResolveInterruption(id string) error {
	return nil
}

func interactiveStateOpsLogJSON(ops []interactive.StateOp) string {
	data, err := json.Marshal(ops)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(data)
}

func interactiveMessageListSummary(messages []*schema.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, interactiveMinInt(len(messages), 8)+1)
	if len(messages) <= 8 {
		for i, msg := range messages {
			parts = append(parts, interactiveMessageSummary(i, msg))
		}
	} else {
		for i := 0; i < 4; i++ {
			parts = append(parts, interactiveMessageSummary(i, messages[i]))
		}
		parts = append(parts, fmt.Sprintf("... omitted=%d ...", len(messages)-8))
		for i := len(messages) - 4; i < len(messages); i++ {
			parts = append(parts, interactiveMessageSummary(i, messages[i]))
		}
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(messages), strings.Join(parts, "; "))
}

func interactiveMessageSummary(index int, msg *schema.Message) string {
	if msg == nil {
		return fmt.Sprintf("%d:<nil>", index)
	}
	return fmt.Sprintf("%d:%s(%s)", index, msg.Role, interactivePartSummary(msg.Content))
}

func interactivePartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + interactiveBoolString(s != ""),
		"bytes=" + fmt.Sprint(len(s)),
		"chars=" + fmt.Sprint(utf8.RuneCountInString(s)),
		"lines=" + fmt.Sprint(interactiveLineCount(s)),
		"sha=" + interactiveShortSHA256(s),
	}, ",")
}

func interactiveBoolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func interactiveLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func interactiveShortSHA256(s string) string {
	if s == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func interactiveMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
