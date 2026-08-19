package app

import (
	"testing"

	"denova/config"
	"denova/internal/interactive"
)

func TestStartInteractiveNarrativeMemoryTaskGatedByMode(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "门控", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "行动",
		Narrative: "叙事",
	})
	if err != nil {
		t.Fatal(err)
	}
	// manual(默认)不启动任何任务,静默返回。
	startInteractiveNarrativeMemoryTask(&config.Config{NarrativeMemoryPublishMode: config.NarrativeMemoryPublishModeManual}, nil, turn)
	startInteractiveNarrativeMemoryTask(&config.Config{NarrativeMemoryPublishMode: config.NarrativeMemoryPublishModeEveryTurn}, nil, turn)
	// conversation 为 nil 时也静默返回(不 panic)。
	if err := func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = errRecovery(recovered)
			}
		}()
		startInteractiveNarrativeMemoryTask(&config.Config{NarrativeMemoryPublishMode: config.NarrativeMemoryPublishModeEveryTurn}, nil, turn)
		return nil
	}(); err != nil {
		t.Fatal(err)
	}
}

func errRecovery(recovered any) error {
	if err, ok := recovered.(error); ok {
		return err
	}
	return nil
}

func TestOpenMemoryPromisesListsOnlyOpen(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "伏笔目录", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "埋伏笔",
		Narrative: "岚警告林舟不要提剑的来历。",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "兑现",
		Narrative: "岚说出了剑的来历。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: first.ID,
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "mem_p1", Kind: interactive.MemoryKindPromise, Subject: "剑的来历", Text: "来历未揭示。", Evidence: "不要提", ValidFrom: first.ID, Status: interactive.MemoryStatusOpen},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 第一回合视角:该伏笔悬置,应列出。
	promises, err := openMemoryPromises(store, story.ID, "main", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(promises) != 1 || promises[0] == "" {
		t.Fatalf("promises: %#v", promises)
	}

	// 兑现后(第二回合视角=latest):promise 记录带 paid 状态,不再列出。
	if _, err := store.AppendNarrativeMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: second.ID,
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "mem_p2", Kind: interactive.MemoryKindPromise, Subject: "剑的来历", Text: "来历已揭示。", Evidence: "说出了", ValidFrom: second.ID, Status: interactive.MemoryStatusPaid, ValidTo: second.ID},
		},
	}); err != nil {
		t.Fatal(err)
	}
	promises, err = openMemoryPromises(store, story.ID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(promises) != 0 {
		t.Fatalf("paid promise should not be listed: %#v", promises)
	}
}
