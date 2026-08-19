package app

import (
	"context"
	"log"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/interactive"
)

// narrativeMemoryExtractionTimeout bounds one background memory extraction
// call. The task is best-effort: timeouts only skip that turn's extraction.
const narrativeMemoryExtractionTimeout = 45 * time.Second

// startInteractiveNarrativeMemoryTask schedules the narrative-memory
// extraction for one committed turn on the branch's "memory" lane. It never
// blocks the interactive run: failures are logged and surface later in the
// memory inspector coverage stats.
func startInteractiveNarrativeMemoryTask(cfg *config.Config, conversation *interactiveConversation, turn interactive.TurnEvent) {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return
	}
	if config.NormalizeNarrativeMemoryPublishMode(cfg.NarrativeMemoryPublishMode) != config.NarrativeMemoryPublishModeEveryTurn {
		return
	}
	tasks := directorTasksForConversation(conversation)
	tasks.GoKeyed(interactiveBranchMaintenanceKey(conversation, turn.BranchID, "memory"), func(ctx context.Context) {
		storyID := conversation.storyID
		if err := publishNarrativeMemoryForTurn(ctx, cfg, conversation.store, storyID, turn); err != nil {
			log.Printf("[narrative-memory] publish failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
		}
	})
}

// publishNarrativeMemoryForTurn extracts typed memory records for one turn
// and appends them as a narrative_memory event. Re-extraction for the same
// turn is naturally idempotent at projection time (larger epoch wins).
func publishNarrativeMemoryForTurn(ctx context.Context, cfg *config.Config, store *interactive.Store, storyID string, turn interactive.TurnEvent) error {
	_ = ctx // request cancellation must not own background memory work
	if store == nil || cfg == nil || strings.TrimSpace(turn.ID) == "" {
		return nil
	}
	openPromises, err := openMemoryPromises(store, storyID, turn.BranchID, turn.ID)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), narrativeMemoryExtractionTimeout)
	defer cancel()
	started := time.Now()
	result, err := agent.GenerateNarrativeMemory(runCtx, cfg, agent.MemoryExtractionInput{
		StoryID:      storyID,
		BranchID:     turn.BranchID,
		Turn:         turn,
		OpenPromises: openPromises,
	})
	trace := &interactive.NarrativeMemoryTrace{
		DurationMs: time.Since(started).Milliseconds(),
	}
	if err != nil {
		trace.SkippedReason = "error: " + err.Error()
		// 落一条空 Trace 事件让覆盖率统计能看见失败,不产生记录。
		_, appendErr := store.AppendNarrativeMemory(storyID, turn.BranchID, interactive.NarrativeMemoryEvent{
			SourceTurnID: turn.ID,
			Records:      nil,
			Trace:        trace,
		})
		if appendErr != nil {
			log.Printf("[narrative-memory] persist failure trace failed story_id=%s turn_id=%s err=%v", storyID, turn.ID, appendErr)
		}
		return err
	}
	trace.DroppedRecords = result.Dropped
	if len(result.Records) == 0 && len(result.Dropped) == 0 {
		trace.SkippedReason = "no_records"
	}
	event, err := store.AppendNarrativeMemory(storyID, turn.BranchID, interactive.NarrativeMemoryEvent{
		SourceTurnID: turn.ID,
		Records:      result.Records,
		Trace:        trace,
	})
	if err != nil {
		return err
	}
	log.Printf("[narrative-memory] published story_id=%s branch_id=%s turn_id=%s event_id=%s records=%d dropped=%d", storyID, turn.BranchID, turn.ID, event.ID, len(result.Records), len(result.Dropped))
	return nil
}

// openMemoryPromises lists currently-open promise records as one-line
// descriptions so the extractor can mark payoffs in this turn. Per subject
// the latest record wins: an earlier open promise whose newer record is paid
// (or carries valid_to) counts as closed.
func openMemoryPromises(store *interactive.Store, storyID, branchID, beforeTurnID string) ([]string, error) {
	view, err := store.BrowseStoryMemory(storyID, branchID, interactive.MemoryKindPromise, beforeTurnID)
	if err != nil {
		return nil, err
	}
	latestBySubject := map[string]interactive.MemoryLibraryEntry{}
	for _, entry := range view.Entries {
		latest, exists := latestBySubject[entry.Subject]
		if !exists || entry.ValidFrom >= latest.ValidFrom {
			latestBySubject[entry.Subject] = entry
		}
	}
	promises := make([]string, 0, len(latestBySubject))
	for _, entry := range latestBySubject {
		if entry.ValidTo != "" || entry.Status == interactive.MemoryStatusPaid {
			continue
		}
		promises = append(promises, entry.Subject+" "+entry.Text+" (turn "+entry.ValidFrom+")")
	}
	return promises, nil
}
