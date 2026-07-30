package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode/utf8"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

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

type interactiveTurnHistory struct {
	PreviousSummary string
	Turns           []interactive.StoryModelTurn
	PreviousCount   int
	OmittedCount    int
}

const (
	interactiveStoryRuntimeContextBytes = interactive.DirectorContextMaxBytes
	interactiveDirectorContextBytes     = interactive.DirectorContextMaxBytes
	// The raw resident bodies keep their 1 MiB safety ceiling. This additional
	// bounded allowance covers deterministic Lore metadata and the standalone
	// message wrapper while still constraining the exact model-visible fragment.
	interactiveResidentLoreMessageMaxBytes = book.ResidentLoreSafetyMaxBytes + interactive.DirectorContextMaxBytes
)

func buildInteractiveTurnHistory(turns []interactive.TurnEvent) interactiveTurnHistory {
	return interactiveTurnHistory{Turns: interactiveStoryModelTurns(turns)}
}

func buildInteractiveModelVisibleTurnHistory(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) interactiveTurnHistory {
	return buildInteractiveTurnHistoryWithCompaction(turns, compaction, retainedTurnsForInteractiveCompaction(compaction))
}

func buildInteractiveModelVisibleHistory(history interactive.StoryModelHistory, compaction *interactive.ContextCompactionEvent) interactiveTurnHistory {
	result := interactiveTurnHistory{Turns: append([]interactive.StoryModelTurn(nil), history.Turns...)}
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		result.PreviousCount = compaction.SourceTurnCount
		result.OmittedCount = compaction.SourceTurnCount
	}
	return result
}

func retainedTurnsForInteractiveCompaction(compaction *interactive.ContextCompactionEvent) int {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return 0
	}
	if compaction.RetainedTurns > 0 {
		return compaction.RetainedTurns
	}
	return config.DefaultContextCompactionRetainedTurns
}

func buildInteractiveTurnHistoryWithCompaction(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent, retainedTurns int) interactiveTurnHistory {
	return buildInteractiveTurnHistoryWindowWithCompaction(turns, 0, compaction, retainedTurns)
}

func buildInteractiveTurnHistoryWindowWithCompaction(turns []interactive.TurnEvent, turnStart int, compaction *interactive.ContextCompactionEvent, retainedTurns int) interactiveTurnHistory {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return buildInteractiveTurnHistory(turns)
	}
	if retainedTurns <= 0 {
		retainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	if retainedTurns > config.MaxContextCompactionRetainedTurns {
		retainedTurns = config.MaxContextCompactionRetainedTurns
	}
	sourceCount := compaction.SourceTurnCount - turnStart
	if sourceCount < 0 {
		sourceCount = 0
	}
	if sourceCount > len(turns) {
		sourceCount = len(turns)
	}
	sourceTail := append([]interactive.TurnEvent(nil), turns[:sourceCount]...)
	if len(sourceTail) > retainedTurns {
		sourceTail = sourceTail[len(sourceTail)-retainedTurns:]
	}
	appended := append([]interactive.TurnEvent(nil), turns[sourceCount:]...)
	retained := make([]interactive.TurnEvent, 0, len(sourceTail)+len(appended))
	retained = append(retained, sourceTail...)
	retained = append(retained, appended...)
	return interactiveTurnHistory{
		PreviousSummary: "",
		Turns:           interactiveStoryModelTurns(retained),
		PreviousCount:   compaction.SourceTurnCount,
		OmittedCount:    compaction.SourceTurnCount,
	}
}

func interactiveStoryModelTurns(turns []interactive.TurnEvent) []interactive.StoryModelTurn {
	if len(turns) == 0 {
		return nil
	}
	result := make([]interactive.StoryModelTurn, 0, len(turns))
	for _, turn := range turns {
		result = append(result, interactive.StoryModelTurn{
			ID: turn.ID, BranchID: turn.BranchID, Ts: turn.Ts, User: turn.User, Narrative: turn.Narrative,
			ModelContextMessages: interactive.CloneModelContextMessages(turn.ModelContextMessages),
		})
	}
	return result
}

func interactiveSnapshotTurnCount(snapshot interactive.Snapshot) int {
	if snapshot.TurnCount >= len(snapshot.Turns) {
		return snapshot.TurnCount
	}
	return len(snapshot.Turns)
}

func interactiveModelCompaction(snapshot interactive.Snapshot) *interactive.ContextCompactionEvent {
	compaction := snapshot.ContextCompaction
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return nil
	}
	turnCount := interactiveSnapshotTurnCount(snapshot)
	if compaction.SourceTurnCount < 0 || compaction.SourceTurnCount > turnCount {
		return nil
	}
	return compaction
}

func interactiveModelHistoryRange(snapshot interactive.Snapshot) (startTurn, endTurn int, compaction *interactive.ContextCompactionEvent) {
	endTurn = interactiveSnapshotTurnCount(snapshot)
	compaction = interactiveModelCompaction(snapshot)
	if compaction != nil {
		startTurn = max(0, compaction.SourceTurnCount-retainedTurnsForInteractiveCompaction(compaction))
	}
	return startTurn, endTurn, compaction
}

func (c *interactiveConversation) modelHistoryForCycle(storyCtx interactive.StoryContext) (interactive.StoryModelHistory, *interactive.ContextCompactionEvent, error) {
	if c == nil || c.store == nil {
		return interactive.StoryModelHistory{}, nil, fmt.Errorf("互动故事不存在")
	}
	branchID := storyCtx.Snapshot.BranchID
	startTurn, turnCount, compaction := interactiveModelHistoryRange(storyCtx.Snapshot)
	compactionID := ""
	if compaction != nil {
		compactionID = compaction.ID
	}
	branchHead := ""
	if branch, ok := storyCtx.Meta.Branches[branchID]; ok {
		branchHead = branch.Head
	}
	cacheKey := strings.Join([]string{
		c.storyID, branchID, branchHead, fmt.Sprint(storyCtx.Snapshot.ContextRevision), fmt.Sprint(startTurn), fmt.Sprint(turnCount), compactionID,
	}, "\x00")
	c.mu.Lock()
	if c.modelHistory != nil && c.modelHistoryKey == cacheKey {
		cached := *c.modelHistory
		c.mu.Unlock()
		return cached, compaction, nil
	}
	c.mu.Unlock()

	history, err := c.store.ReadModelHistory(c.storyID, interactive.StoryModelHistoryQuery{
		BranchID: branchID, StartTurn: startTurn, EndTurn: turnCount,
	})
	if err != nil {
		return interactive.StoryModelHistory{}, nil, err
	}
	c.mu.Lock()
	c.modelHistoryKey = cacheKey
	cached := history
	c.modelHistory = &cached
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] loaded model history story_id=%s branch_id=%s start_turn=%d end_turn=%d total_turns=%d model_turns=%d checkpoint_id=%s",
		c.storyID, branchID, history.StartTurn, history.EndTurn, history.TotalTurns, len(history.Turns), compactionID,
	)
	return history, compaction, nil
}

func formatInteractiveTurnHistory(turns []interactive.StoryModelTurn, emptyMessage string) string {
	if len(turns) == 0 {
		return emptyMessage
	}
	var sb strings.Builder
	for i, turn := range turns {
		idx := i + 1
		fmt.Fprintf(&sb, "第 %d 回合用户行动：%s\n", idx, strings.TrimSpace(turn.User))
		fmt.Fprintf(&sb, "第 %d 回合剧情：%s\n\n", idx, strings.TrimSpace(turn.Narrative))
	}
	return strings.TrimSpace(sb.String())
}

func formatInteractiveTurnHistoryWithCheckpoint(turnHistory interactiveTurnHistory, compaction *interactive.ContextCompactionEvent, emptyMessage string) string {
	var sb strings.Builder
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		sb.WriteString("[历史上下文检查点]\n")
		sb.WriteString(agents.NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary).Content)
		sb.WriteString("\n\n")
	}
	if len(turnHistory.Turns) > 0 {
		sb.WriteString(formatInteractiveTurnHistory(turnHistory.Turns, emptyMessage))
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		return emptyMessage
	}
	return result
}

func interactiveMessageListSummary(messages []*agents.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	const edgeCount = 4
	if len(messages) <= edgeCount*2 {
		parts := make([]string, 0, len(messages))
		for i, msg := range messages {
			parts = append(parts, interactiveMessageSummary(i, len(messages), msg))
		}
		return fmt.Sprintf("count=%d parts=[%s]", len(messages), strings.Join(parts, "; "))
	}
	// Context can contain thousands of messages before its first checkpoint.
	// Keep diagnostics useful without turning the log itself into an unbounded
	// second copy of model-visible history.
	parts := make([]string, 0, edgeCount*2+1)
	for i := 0; i < edgeCount; i++ {
		parts = append(parts, interactiveMessageSummary(i, len(messages), messages[i]))
	}
	parts = append(parts, fmt.Sprintf("... omitted=%d ...", len(messages)-edgeCount*2))
	for i := len(messages) - edgeCount; i < len(messages); i++ {
		parts = append(parts, interactiveMessageSummary(i, len(messages), messages[i]))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(messages), strings.Join(parts, "; "))
}

func interactiveMessageSummary(index, total int, msg *agents.Message) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
