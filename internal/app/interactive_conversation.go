package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"nova/internal/book"
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
	mu               sync.Mutex
	lastTurn         *interactive.TurnEvent
	lastStateReady   bool
	lastSources      string
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
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	stateJSON, err := json.MarshalIndent(storyCtx.Snapshot.State, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化互动状态失败: %w", err)
	}
	loreItems := c.loreContext()
	characters := ""
	worldBuilding := ""
	if loreItems == "" {
		characters = c.readSettingFile("characters.md")
		worldBuilding = c.readSettingFile("world-building.md")
	}
	contextMessage := prompts.InteractiveStoryContext(prompts.InteractiveStoryPromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		BranchID:          storyCtx.Snapshot.BranchID,
		ReplyTargetChars:  c.replyTargetChars,
		Characters:        characters,
		WorldBuilding:     worldBuilding,
		LoreItems:         loreItems,
		SnapshotStateJSON: string(stateJSON),
	})
	if choices := pendingHotStateChoices(storyCtx.Snapshot); len(choices) > 0 {
		contextMessage += "\n\n" + formatTemporaryActionSpace(choices)
	}
	history := make([]*schema.Message, 0, len(storyCtx.Snapshot.Turns)*2+2)
	history = append(history, schema.UserMessage(contextMessage))
	for _, turn := range storyCtx.Snapshot.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, teller.RandomEventRate)))
	sourceSummary := interactiveStorySourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, loreItems, characters, worldBuilding, string(stateJSON), storyCtx.Snapshot.Turns, agentMessage)
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s teller_slots=%s teller_turn_context=%s random_event_rate=%.2f characters=%s world_building=%s snapshot_state=%s turns=%d history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		teller.RandomEventRate,
		interactivePartSummary(firstNonEmpty(loreItems, characters)),
		interactivePartSummary(worldBuilding),
		interactivePartSummary(string(stateJSON)),
		len(storyCtx.Snapshot.Turns),
		interactiveMessageListSummary(history),
		interactivePartSummary(history[len(history)-1].Content),
		sourceSummary,
	)
	return history, nil
}

func (c *interactiveConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSources
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *interactiveConversation) AppendAssistantWithThinking(content, thinking string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	log.Printf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content)
	narrative, ops, hotState, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	if hotState == nil {
		hotState = fallbackHotStateFromNarrative(narrative)
		log.Printf("[interactive-agent] hot state missing, generated fallback story_id=%s branch_id=%s choices=%d", c.storyID, c.branchID, hotStateChoiceCount(hotState))
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q hot_state_choices=%d ops=%s", c.storyID, c.branchID, narrative, hotStateChoiceCount(hotState), interactiveStateOpsLogJSON(ops))
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:  c.branchID,
		User:      c.user,
		Narrative: narrative,
		Thinking:  thinking,
		Ops:       ops,
		HotState:  hotState,
	})
	if err == nil {
		c.mu.Lock()
		c.lastTurn = &turn
		c.lastStateReady = len(ops) > 0
		c.mu.Unlock()
	}
	return err
}

func (c *interactiveConversation) LastTurnForState() (interactive.TurnEvent, bool, bool) {
	if c == nil {
		return interactive.TurnEvent{}, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastTurn == nil {
		return interactive.TurnEvent{}, false, false
	}
	return *c.lastTurn, c.lastStateReady, true
}

func (c *interactiveConversation) BuildStateInstruction(turn interactive.TurnEvent) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	stateJSON, err := json.MarshalIndent(storyCtx.Snapshot.State, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化互动状态失败: %w", err)
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	instruction := prompts.InteractiveStateInstruction(prompts.InteractiveStatePromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		StoryTellerMemory: teller.PromptForTargets("state_memory"),
		BranchID:          storyCtx.Snapshot.BranchID,
		Characters:        c.readSettingFile("characters.md"),
		WorldBuilding:     c.readSettingFile("world-building.md"),
		LoreItems:         c.loreContext(),
		SnapshotStateJSON: string(stateJSON),
		UserAction:        turn.User,
		Narrative:         turn.Narrative,
	})
	log.Printf(
		"[interactive-state-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s teller_slots=%s sources=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		interactiveTellerSlotSummary(teller, "state_memory"),
		interactiveStateSourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, c.loreContext(), c.readSettingFile("characters.md"), c.readSettingFile("world-building.md"), string(stateJSON), turn.User, turn.Narrative),
		interactivePartSummary(instruction),
	)
	return instruction, nil
}

func (c *interactiveConversation) teller(tellerID string) interactive.Teller {
	return loadInteractiveTeller(c.novaDir, tellerID)
}

func loadInteractiveTeller(novaDir, tellerID string) interactive.Teller {
	if novaDir == "" {
		return interactive.Teller{}
	}
	teller, err := interactive.NewTellerLibrary(novaDir).Get(tellerID)
	if err == nil {
		return teller
	}
	log.Printf("[interactive-agent] load teller failed id=%s err=%v", tellerID, err)
	fallback, fallbackErr := interactive.NewTellerLibrary(novaDir).Get("classic")
	if fallbackErr != nil {
		log.Printf("[interactive-agent] load fallback teller failed err=%v", fallbackErr)
		return interactive.Teller{}
	}
	return fallback
}

func interactiveStoryTellerSystemInput(teller interactive.Teller) prompts.InteractiveStorySystemInstructionInput {
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           teller.ID,
		StoryTellerName:         teller.Name,
		StoryTellerDescription:  teller.Description,
		StoryTellerSystemPrompt: teller.PromptForTargets("system"),
	}
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

func (c *interactiveConversation) loreContext() string {
	if c.workspace == "" {
		return ""
	}
	context, err := book.NewLoreStore(c.workspace).ContextMarkdown()
	if err != nil {
		log.Printf("[interactive-agent] load lore context failed workspace=%s err=%v", c.workspace, err)
		return ""
	}
	return context
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

func hotStateChoiceCount(hot *interactive.HotState) int {
	if hot == nil {
		return 0
	}
	return len(hot.Choices)
}

func fallbackHotStateFromNarrative(narrative string) *interactive.HotState {
	narrative = strings.TrimSpace(narrative)
	if narrative == "" {
		return nil
	}
	return &interactive.HotState{Choices: []string{
		"我继续观察当前场景，寻找新的线索。",
		"我询问在场角色，确认他们知道什么。",
		"我先检查身边可用的物品、危险和退路。",
	}}
}

func pendingHotStateChoices(snapshot interactive.Snapshot) []string {
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.StateStatus != "pending" || snapshot.CurrentTurn.HotState == nil {
		return nil
	}
	return snapshot.CurrentTurn.HotState.Choices
}

func formatTemporaryActionSpace(choices []string) string {
	var sb strings.Builder
	sb.WriteString("## 临时行动空间\n")
	sb.WriteString("上一回合的正式状态仍在同步中，以下是主 Agent 快速生成的可行动候选；它们只用于辅助理解下一步，不替代用户本轮明确输入。\n")
	for i, choice := range choices {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		fmt.Fprintf(&sb, "%d. %s\n", i+1, choice)
	}
	return strings.TrimSpace(sb.String())
}

type interactiveContextSource struct {
	Source  string
	Title   string
	Content string
	Note    string
}

func interactiveStorySourceSummary(title, origin string, teller interactive.Teller, loreItems, characters, worldBuilding, snapshotState string, turns []interactive.TurnEvent, userAction string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(loreItems) != "" {
		parts = append(parts, interactiveContextSource{Source: "资料库", Title: ".nova/lore/items.json", Content: loreItems})
	} else {
		parts = append(parts,
			interactiveContextSource{Source: "设定文件", Title: "setting/characters.md", Content: characters, Note: "资料库为空时回退注入"},
			interactiveContextSource{Source: "设定文件", Title: "setting/world-building.md", Content: worldBuilding, Note: "资料库为空时回退注入"},
		)
	}
	parts = append(parts, interactiveContextSource{Source: "互动状态", Title: "当前快照 JSON", Content: snapshotState})
	for i, turn := range turns {
		parts = append(parts,
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合用户行动", i+1), Content: turn.User},
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合剧情", i+1), Content: turn.Narrative},
		)
	}
	parts = append(parts, interactiveContextSource{Source: "本轮行动", Title: "当前用户行动", Content: userAction})
	return interactiveContextSourceListSummary(parts)
}

func interactiveStateSourceSummary(title, origin string, teller interactive.Teller, loreItems, characters, worldBuilding, snapshotState, userAction, narrative string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "state_memory")...)
	if strings.TrimSpace(loreItems) != "" {
		parts = append(parts, interactiveContextSource{Source: "资料库", Title: ".nova/lore/items.json", Content: loreItems})
	} else {
		parts = append(parts,
			interactiveContextSource{Source: "设定文件", Title: "setting/characters.md", Content: characters, Note: "资料库为空时回退注入"},
			interactiveContextSource{Source: "设定文件", Title: "setting/world-building.md", Content: worldBuilding, Note: "资料库为空时回退注入"},
		)
	}
	parts = append(parts,
		interactiveContextSource{Source: "互动状态", Title: "当前快照 JSON", Content: snapshotState},
		interactiveContextSource{Source: "本轮行动", Title: "用户行动", Content: userAction},
		interactiveContextSource{Source: "本轮剧情", Title: "Agent 正文", Content: narrative},
	)
	return interactiveContextSourceListSummary(parts)
}

func interactiveTellerSlotSources(teller interactive.Teller, targets ...string) []interactiveContextSource {
	allowed := make(map[string]bool, len(targets))
	for _, target := range targets {
		allowed[target] = true
	}
	parts := []interactiveContextSource{}
	for _, slot := range teller.Slots {
		if !slot.Enabled || !allowed[slot.Target] || strings.TrimSpace(slot.Content) == "" {
			continue
		}
		parts = append(parts, interactiveContextSource{
			Source:  "讲述者注入规则",
			Title:   fmt.Sprintf("%s（%s）", slot.Name, slot.Target),
			Content: slot.Content,
			Note:    "teller=" + teller.ID,
		})
	}
	return parts
}

func interactiveTellerSlotSummary(teller interactive.Teller, targets ...string) string {
	sources := interactiveTellerSlotSources(teller, targets...)
	if len(sources) == 0 {
		return "count=0"
	}
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Title)
	}
	return fmt.Sprintf("count=%d names=%q", len(names), names)
}

func interactiveContextSourceListSummary(parts []interactiveContextSource) string {
	if len(parts) == 0 {
		return "count=0"
	}
	items := make([]string, 0, len(parts))
	for i, part := range parts {
		content := strings.TrimSpace(part.Content)
		if content == "" {
			continue
		}
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			fmt.Sprintf("bytes=%d", len(content)),
			fmt.Sprintf("chars=%d", utf8.RuneCountInString(content)),
			"preview=" + strconv.Quote(interactiveSafePreview(content, 100)),
		}
		if part.Note != "" {
			fields = append(fields, "note="+strconv.Quote(part.Note))
		}
		items = append(items, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(items), strings.Join(items, "; "))
}

func interactiveMessageListSummary(messages []*schema.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(messages))
	for i, msg := range messages {
		parts = append(parts, interactiveMessageSummary(i, len(messages), msg))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(messages), strings.Join(parts, "; "))
}

func interactiveMessageSummary(index, total int, msg *schema.Message) string {
	if msg == nil {
		return fmt.Sprintf("%d:<nil>", index)
	}
	source := "互动上下文"
	if index > 0 && index < total-1 {
		source = "历史回合"
	}
	if index == total-1 {
		source = "本轮行动指令"
	}
	return fmt.Sprintf("%d:source=%s role=%s(%s)", index, source, msg.Role, interactivePartSummary(msg.Content))
}

func interactivePartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + interactiveBoolString(s != ""),
		"bytes=" + fmt.Sprint(len(s)),
		"chars=" + fmt.Sprint(utf8.RuneCountInString(s)),
		"lines=" + fmt.Sprint(interactiveLineCount(s)),
		"sha=" + interactiveShortSHA256(s),
		"preview=" + strconv.Quote(interactiveSafePreview(s, 80)),
	}, ",")
}

func interactiveSafePreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
