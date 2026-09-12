package interactiveapp

import (
	"context"
	"crypto/sha256"
	"denova/internal/book/lore"
	"encoding/hex"
	"fmt"
	agent "github.com/alfredxw/denova/agent"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func (c *Conversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("interactive conversation is unavailable")
	}
	c.mu.Lock()
	playerInputID := c.acceptedPlayerInputID
	c.mu.Unlock()
	interruption, err := c.store.MarkTurnInterrupted(
		c.storyID, c.branchID, playerInputID, userMessage, assistantContent, reason,
	)
	if err != nil {
		return err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[interactive-agent] persisted paused turn story_id=%s branch_id=%s interruption_id=%s player_input_id=%s partial_bytes=%d",
		c.storyID, c.branchID, interruption.ID, playerInputID, len(assistantContent),
	))
	return nil
}

func (c *Conversation) PendingInterruption() *session.Interruption {
	if c == nil || c.store == nil {
		return nil
	}
	interruption, err := c.store.PendingTurnInterruption(c.storyID, c.branchID)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf(
			"[interactive-agent] load paused turn failed story_id=%s branch_id=%s err=%v",
			c.storyID, c.branchID, err,
		))
		return nil
	}
	if interruption == nil {
		return nil
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, interruption.Ts)
	return &session.Interruption{
		ID: interruption.ID, Status: session.InterruptionPending,
		UserMessage: interruption.UserMessage, AssistantContent: interruption.AssistantContent,
		Reason: interruption.Reason, CreatedAt: createdAt,
	}
}

func (c *Conversation) ResolveInterruption(id string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("interactive conversation is unavailable")
	}
	pending, err := c.store.PendingTurnInterruption(c.storyID, c.branchID)
	if err != nil {
		return err
	}
	if pending != nil && pending.ID != strings.TrimSpace(id) {
		return fmt.Errorf("pending turn interruption changed: have=%s want=%s", pending.ID, strings.TrimSpace(id))
	}
	return nil
}

type interactiveTurnHistory struct {
	Turns []interactive.StoryModelTurn
}

const (
	StoryRuntimeContextMaxBytes = interactive.StoryContextMaxBytes
	// The raw resident bodies keep their 1 MiB safety ceiling. This additional
	// bounded allowance covers deterministic Lore metadata and the standalone
	// message wrapper while still constraining the exact model-visible fragment.
	interactiveResidentLoreMessageMaxBytes = lore.ResidentLoreSafetyMaxBytes + interactive.StoryContextMaxBytes
)

func SnapshotTurnCount(snapshot interactive.Snapshot) int {
	if snapshot.TurnCount >= len(snapshot.Turns) {
		return snapshot.TurnCount
	}
	return len(snapshot.Turns)
}

func (c *Conversation) modelHistoryForCycle(storyCtx interactive.StoryContext) (interactive.StoryModelHistory, *agent.CompactionState, error) {
	if c == nil || c.store == nil {
		return interactive.StoryModelHistory{}, nil, fmt.Errorf("interactive story does not exist")
	}
	branchID := storyCtx.Snapshot.BranchID
	turnCount := SnapshotTurnCount(storyCtx.Snapshot)
	compaction := c.boundAgentCompaction()
	startTurn := 0
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
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[interactive-agent] loaded model history story_id=%s branch_id=%s start_turn=%d end_turn=%d total_turns=%d model_turns=%d checkpoint_id=%s",
		c.storyID, branchID, history.StartTurn, history.EndTurn, history.TotalTurns, len(history.Turns), compactionID,
	))
	return history, compaction, nil
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
	source := "interactive context"
	if index > 0 && index < total-1 {
		source = "historical turn"
	}
	if index == total-1 {
		source = "current turn instruction"
	}
	return fmt.Sprintf("%d:source=%s role=%s(%s)", index, source, msg.Role, PartSummary(msg.Content))
}

func PartSummary(s string) string {
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
