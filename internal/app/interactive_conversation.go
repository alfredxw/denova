package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

type interactiveConversation struct {
	store            *interactive.Store
	novaDir          string
	workspace        string
	cfg              *config.Config
	storyID          string
	branchID         string
	user             string
	replyTargetChars int
	mu               sync.Mutex
	lastTurn         *interactive.TurnEvent
	lastStateReady   bool
	lastSources      string
	displayEvents    []interactive.DisplayEvent
}

func newInteractiveConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *interactiveConversation {
	return &interactiveConversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
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
	turnMemory := buildInteractiveTurnMemory(storyCtx.Snapshot.Turns, teller.ContextPolicy.RecentTurns)
	stateJSON, err := json.MarshalIndent(storyCtx.Snapshot.State, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化互动状态失败: %w", err)
	}
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 12*1024)
	if err != nil {
		log.Printf("[interactive-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	characters := ""
	worldBuilding := ""
	contextMessage := prompts.InteractiveStoryContext(prompts.InteractiveStoryPromptInput{
		Title:                storyCtx.Meta.Title,
		Origin:               storyCtx.Meta.Origin,
		StoryTellerID:        storyCtx.Meta.StoryTellerID,
		BranchID:             storyCtx.Snapshot.BranchID,
		ReplyTargetChars:     c.replyTargetChars,
		Characters:           characters,
		WorldBuilding:        worldBuilding,
		LongTermMemory:       storyMemory,
		SnapshotStateJSON:    string(stateJSON),
		PreviousTurnsSummary: turnMemory.PreviousSummary,
	})
	history := make([]*schema.Message, 0, len(turnMemory.RecentTurns)*2+2)
	history = append(history, schema.UserMessage(contextMessage))
	for _, turn := range turnMemory.RecentTurns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, teller.RandomEventRate)))
	sourceSummary := interactiveStorySourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, characters, worldBuilding, storyMemory, turnMemory, agentMessage)
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s teller_slots=%s teller_turn_context=%s random_event_rate=%.2f characters=%s world_building=%s snapshot_state=%s turns=%d recent_turns=%d compressed_turns=%s history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		teller.RandomEventRate,
		interactivePartSummary(characters),
		interactivePartSummary(worldBuilding),
		interactivePartSummary(storyMemory),
		len(storyCtx.Snapshot.Turns),
		len(turnMemory.RecentTurns),
		interactivePartSummary(turnMemory.PreviousSummary),
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
	narrative, ops, _, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q ops=%s", c.storyID, c.branchID, narrative, interactiveStateOpsLogJSON(ops))
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:      c.branchID,
		User:          c.user,
		Narrative:     narrative,
		Thinking:      thinking,
		DisplayEvents: c.displayEventsSnapshot(),
		Ops:           ops,
	})
	if err == nil {
		c.mu.Lock()
		c.lastTurn = &turn
		c.lastStateReady = false
		c.mu.Unlock()
	}
	return err
}

func (c *interactiveConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil {
		return nil
	}
	role := strings.TrimSpace(event.Role)
	if role == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	// thinking 已作为回合字段持久化；这里仅保留工具卡片，避免刷新后重复展示思考块。
	if role == "thinking" {
		return nil
	}
	if role != "tool_call" && role != "tool_result" {
		return nil
	}
	name := strings.TrimSpace(event.Name)
	content := strings.TrimSpace(event.Content)
	if role == "tool_call" {
		if name == "" {
			name = content
		}
		if name == "" {
			name = "unknown_tool"
		}
		content = name
	}
	status := strings.TrimSpace(event.Status)
	if role == "tool_call" && status == "" {
		status = "running"
	}
	createdAt := ""
	if !event.CreatedAt.IsZero() {
		createdAt = event.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.displayEvents = append(c.displayEvents, interactive.DisplayEvent{
		ID:        strings.TrimSpace(event.ID),
		Role:      role,
		Content:   content,
		Name:      name,
		Status:    status,
		CreatedAt: createdAt,
	})
	return nil
}

func (c *interactiveConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.displayEvents) - 1; i >= 0; i-- {
		event := c.displayEvents[i]
		if event.Role != "tool_call" {
			continue
		}
		if id != "" && event.ID != id {
			continue
		}
		if id == "" && name != "" && event.Name != name {
			continue
		}
		c.displayEvents[i].Status = status
		return nil
	}
	return nil
}

func (c *interactiveConversation) displayEventsSnapshot() []interactive.DisplayEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.displayEvents) == 0 {
		return nil
	}
	result := make([]interactive.DisplayEvent, len(c.displayEvents))
	copy(result, c.displayEvents)
	return result
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
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 12*1024)
	if err != nil {
		log.Printf("[interactive-state-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	instruction := prompts.InteractiveStateInstruction(prompts.InteractiveStatePromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		StoryTellerMemory: teller.PromptForTargets("state_memory"),
		BranchID:          storyCtx.Snapshot.BranchID,
		Characters:        "",
		WorldBuilding:     "",
		LoreItems:         c.loreContext(),
		SnapshotStateJSON: storyMemory,
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
		interactiveStateSourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, c.loreContext(), "", "", storyMemory, turn.User, turn.Narrative),
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

func (c *interactiveConversation) loreContext() string {
	if c.workspace == "" {
		return ""
	}
	context, err := book.NewLoreStore(c.workspace).ProgressiveContextMarkdown()
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

type interactiveContextSource struct {
	Source  string
	Title   string
	Content string
	Note    string
}

type interactiveTurnMemory struct {
	PreviousSummary string
	RecentTurns     []interactive.TurnEvent
	PreviousCount   int
	OmittedCount    int
}

const (
	interactiveMemoryMaxPreviousTurns = 80
	interactiveMemorySummaryMaxBytes  = 16 * 1024
)

func buildInteractiveTurnMemory(turns []interactive.TurnEvent, recentLimit int) interactiveTurnMemory {
	if recentLimit <= 0 {
		recentLimit = 8
	}
	if recentLimit > 30 {
		recentLimit = 30
	}
	if len(turns) <= recentLimit {
		return interactiveTurnMemory{RecentTurns: append([]interactive.TurnEvent(nil), turns...)}
	}
	split := len(turns) - recentLimit
	previous := turns[:split]
	recent := append([]interactive.TurnEvent(nil), turns[split:]...)
	omitted := 0
	if len(previous) > interactiveMemoryMaxPreviousTurns {
		omitted = len(previous) - interactiveMemoryMaxPreviousTurns
		previous = previous[omitted:]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "以下为较早 %d 个回合的有界摘要；完整原文不再进入本轮模型上下文。\n", split)
	if omitted > 0 {
		fmt.Fprintf(&sb, "更早的 %d 个回合已超过摘要上限，仅保留当前窗口前最近的压缩记忆。\n", omitted)
	}
	for i, turn := range previous {
		if sb.Len() >= interactiveMemorySummaryMaxBytes {
			sb.WriteString("- 摘要达到大小上限，剩余较早回合省略。\n")
			break
		}
		turnIndex := omitted + i + 1
		fmt.Fprintf(&sb, "- 第 %d 回合 用户：%s\n  剧情：%s\n", turnIndex, interactiveSafePreview(turn.User, 120), interactiveSafePreview(turn.Narrative, 180))
	}
	return interactiveTurnMemory{
		PreviousSummary: strings.TrimSpace(sb.String()),
		RecentTurns:     recent,
		PreviousCount:   split,
		OmittedCount:    omitted,
	}
}

func interactiveStorySourceSummary(title, origin string, teller interactive.Teller, characters, worldBuilding, snapshotState string, turnMemory interactiveTurnMemory, userAction string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	parts = append(parts, interactiveContextSource{Source: "互动状态", Title: "当前快照 JSON", Content: snapshotState})
	if strings.TrimSpace(turnMemory.PreviousSummary) != "" {
		parts = append(parts, interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("较早 %d 回合压缩摘要", turnMemory.PreviousCount), Content: turnMemory.PreviousSummary, Note: "compressed"})
	}
	for i, turn := range turnMemory.RecentTurns {
		parts = append(parts,
			interactiveContextSource{Source: "最近历史回合", Title: fmt.Sprintf("最近第 %d 回合用户行动", i+1), Content: turn.User},
			interactiveContextSource{Source: "最近历史回合", Title: fmt.Sprintf("最近第 %d 回合剧情", i+1), Content: turn.Narrative},
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
			Source:  "导演注入规则",
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
