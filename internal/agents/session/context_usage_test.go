package session

import "testing"

func TestLatestModelPromptUsageUsesFinalCallAndSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("usage")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID: "run-1", Role: "token_usage", AgentKind: "ide",
		PromptTokens: 30_000, CachedPromptTokens: 20_000, ModelCalls: 2,
		UsageCalls: []TokenUsageCall{
			{Index: 1, PromptTokens: 12_000, CachedPromptTokens: 8_000},
			{Index: 2, PromptTokens: 18_000, CachedPromptTokens: 12_000},
		},
	}); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	prompt, cached, ok := reloaded.LatestModelPromptUsage("ide")
	if !ok || prompt != 18_000 || cached != 12_000 {
		t.Fatalf("latest prompt usage = (%d, %d, %t)", prompt, cached, ok)
	}
	if _, _, ok := reloaded.LatestModelPromptUsage("interactive_story"); ok {
		t.Fatal("usage from another Agent leaked into calibration")
	}
	if err := reloaded.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := reloaded.LatestModelPromptUsage("ide"); ok {
		t.Fatal("usage from before a structural context change remained active")
	}
}
