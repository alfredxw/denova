package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	contextMessage := prompts.InteractiveStoryContext(prompts.InteractiveStoryPromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		StoryTeller:       tellerPrompt,
		BranchID:          storyCtx.Snapshot.BranchID,
		ReplyTargetChars:  c.replyTargetChars,
		Characters:        c.readSettingFile("characters.md"),
		WorldBuilding:     c.readSettingFile("world-building.md"),
		SnapshotStateJSON: string(stateJSON),
	})
	history := make([]*schema.Message, 0, len(storyCtx.Snapshot.Turns)*2+2)
	history = append(history, schema.UserMessage(contextMessage))
	for _, turn := range storyCtx.Snapshot.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage)))
	return history, nil
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *interactiveConversation) AppendAssistantWithThinking(content, thinking string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	narrative, ops, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		if narrative == "" {
			return parseErr
		}
		log.Printf("[interactive-agent] state delta parse skipped story_id=%s branch_id=%s err=%v", c.storyID, c.branchID, parseErr)
	}
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
