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

// narrativeMemoryEmbeddingTimeout bounds one vector indexing call. It is
// shorter than extraction: embedding has no generation phase.
const narrativeMemoryEmbeddingTimeout = 30 * time.Second

// narrativeMemoryGenerator 是叙事记忆抽取的可替换实现,让后台任务的调度、
// 去重与落库能在没有模型调用的情况下被完整覆盖。
type narrativeMemoryGenerator func(context.Context, *config.Config, agent.MemoryExtractionInput) (agent.MemoryExtractionResult, error)

// memoryGeneratorForConversation 返回会话使用的抽取实现,未注入时用真实模型。
func memoryGeneratorForConversation(conversation *interactiveConversation) narrativeMemoryGenerator {
	if conversation != nil && conversation.memoryGenerator != nil {
		return conversation.memoryGenerator
	}
	return agent.GenerateNarrativeMemory
}

// startInteractiveNarrativeMemoryTask schedules the narrative-memory
// extraction for one committed turn on the branch's "memory" lane. It never
// blocks the interactive run: failures are logged and surface later in the
// memory inspector coverage stats.
func startInteractiveNarrativeMemoryTask(cfg *config.Config, conversation *interactiveConversation, turn interactive.TurnEvent) <-chan struct{} {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return nil
	}
	if config.NormalizeNarrativeMemoryPublishMode(cfg.NarrativeMemoryPublishMode) != config.NarrativeMemoryPublishModeEveryTurn {
		return nil
	}
	generate := memoryGeneratorForConversation(conversation)
	tasks := directorTasksForConversation(conversation)
	done, _ := tasks.GoKeyed(interactiveBranchMaintenanceKey(conversation, turn.BranchID, "memory"), func(ctx context.Context) {
		storyID := conversation.storyID
		if err := publishNarrativeMemoryForTurn(ctx, cfg, conversation.store, storyID, turn, generate); err != nil {
			log.Printf("[narrative-memory] publish failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
		}
	})
	return done
}

// startInteractiveCompactionMemoryTask backfills narrative memory for the
// turns a compaction just pushed out of the model context.
//
// The turns are not lost — compaction only rewrites what the model sees, the
// event log keeps every turn forever. So this is not a rescue: it is a cost
// decision. on_compaction trades per-turn extraction for one batch at the
// moment the context actually overflows and memory starts earning its keep.
//
// Runs on the same "memory" lane as per-turn extraction, so the two can never
// race on the branch head.
func startInteractiveCompactionMemoryTask(cfg *config.Config, conversation *interactiveConversation, branchID string, turns []interactive.TurnEvent) <-chan struct{} {
	if conversation == nil || conversation.store == nil || cfg == nil || len(turns) == 0 {
		return nil
	}
	if config.NormalizeNarrativeMemoryPublishMode(cfg.NarrativeMemoryPublishMode) != config.NarrativeMemoryPublishModeOnCompact {
		return nil
	}
	store := conversation.store
	storyID := conversation.storyID
	generate := memoryGeneratorForConversation(conversation)
	tasks := directorTasksForConversation(conversation)
	done, _ := tasks.GoKeyed(interactiveBranchMaintenanceKey(conversation, branchID, "memory"), func(ctx context.Context) {
		_ = ctx // request cancellation must not own background memory work
		covered, err := store.NarrativeMemoryCoveredTurns(storyID, branchID)
		if err != nil {
			log.Printf("[narrative-memory] compaction backfill aborted story_id=%s branch_id=%s err=%v", storyID, branchID, err)
			return
		}
		published := 0
		for _, turn := range turns {
			if covered[turn.ID] {
				continue
			}
			if err := publishNarrativeMemoryForTurn(context.Background(), cfg, store, storyID, turn, generate); err != nil {
				log.Printf("[narrative-memory] compaction backfill failed story_id=%s turn_id=%s err=%v", storyID, turn.ID, err)
				continue
			}
			published++
		}
		log.Printf("[narrative-memory] compaction backfill done story_id=%s branch_id=%s turns=%d published=%d", storyID, branchID, len(turns), published)
	})
	return done
}

// publishNarrativeMemoryForTurn extracts typed memory records for one turn
// and appends them as a narrative_memory event. Re-extraction for the same
// turn is naturally idempotent at projection time (larger epoch wins).
func publishNarrativeMemoryForTurn(ctx context.Context, cfg *config.Config, store *interactive.Store, storyID string, turn interactive.TurnEvent, generate narrativeMemoryGenerator) error {
	_ = ctx // request cancellation must not own background memory work
	if store == nil || cfg == nil || strings.TrimSpace(turn.ID) == "" {
		return nil
	}
	if generate == nil {
		generate = agent.GenerateNarrativeMemory
	}
	openPromises, err := openMemoryPromises(store, storyID, turn.BranchID, turn.ID)
	if err != nil {
		return err
	}
	// 名册取本回合之前的实体:抽取器要对齐到已有写法,不该看见自己正在产出的记录。
	roster, err := store.StoryEntityRoster(storyID, turn.BranchID, turn.ID, interactive.DefaultMemoryRosterLimit)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), narrativeMemoryExtractionTimeout)
	defer cancel()
	started := time.Now()
	result, err := generate(runCtx, cfg, agent.MemoryExtractionInput{
		StoryID:      storyID,
		BranchID:     turn.BranchID,
		Turn:         turn,
		OpenPromises: openPromises,
		Roster:       roster,
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
	indexNarrativeMemoryVectors(cfg, store, storyID, event.Records)
	return nil
}

// indexNarrativeMemoryVectors 为刚落盘的记录补算向量并写入侧车,供后续检索的
// 向量召回使用。整段是可选增强:未配置 embedding 模型、调用失败或超时都只记
// 日志——记忆事实已经写入事件日志,缺向量只会让检索退回纯关键词路径。
func indexNarrativeMemoryVectors(cfg *config.Config, store *interactive.Store, storyID string, records []interactive.NarrativeMemoryRecord) {
	if len(records) == 0 {
		return
	}
	embedder := agent.NewNarrativeMemoryEmbedder(cfg)
	if embedder == nil {
		return
	}
	texts := make([]string, 0, len(records))
	recordIDs := make([]string, 0, len(records))
	for _, record := range records {
		text := interactive.MemoryVectorText(record)
		if strings.TrimSpace(text) == "" || strings.TrimSpace(record.ID) == "" {
			continue
		}
		texts = append(texts, text)
		recordIDs = append(recordIDs, record.ID)
	}
	if len(texts) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), narrativeMemoryEmbeddingTimeout)
	defer cancel()
	vectors, err := embedder.EmbedMemoryTexts(ctx, texts)
	if err != nil {
		log.Printf("[narrative-memory] embedding failed story_id=%s records=%d err=%v", storyID, len(texts), err)
		return
	}
	indexed := make(map[string][]float32, len(recordIDs))
	for i, recordID := range recordIDs {
		if i < len(vectors) {
			indexed[recordID] = vectors[i]
		}
	}
	if err := store.AppendMemoryVectors(storyID, embedder.EmbeddingModelID(), indexed); err != nil {
		log.Printf("[narrative-memory] persist vectors failed story_id=%s err=%v", storyID, err)
		return
	}
	log.Printf("[narrative-memory] indexed vectors story_id=%s model=%s records=%d", storyID, embedder.EmbeddingModelID(), len(indexed))
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
