package interactive

import "testing"

func TestSpeechPreferencesSurviveJournalReopenWithoutChangingTurns(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Speech"})
	if err != nil {
		t.Fatal(err)
	}
	if story.SpeechSettings != (StorySpeechSettings{Mode: "all"}) {
		t.Fatalf("unexpected defaults: %#v", story.SpeechSettings)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "Go", Narrative: "她说：“你好。”"})
	if err != nil {
		t.Fatal(err)
	}
	want := StorySpeechSettings{AutoRead: true, Mode: "quoted", IgnoreAsterisks: true}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{SpeechSettings: &want}); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	meta, _, err := reopened.readStoryLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SpeechSettings != want {
		t.Fatalf("journal lost preferences: %#v", meta.SpeechSettings)
	}
	snapshot, err := reopened.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID || snapshot.CurrentTurn.Narrative != turn.Narrative {
		t.Fatal("speech settings changed the story")
	}
	invalid := StorySpeechSettings{Mode: "speaker-detection"}
	if _, err := reopened.UpdateStory(story.ID, UpdateStoryRequest{SpeechSettings: &invalid}); err == nil {
		t.Fatal("invalid content mode accepted")
	}
}
