package interactive

import (
	"strings"
	"testing"
)

func TestGeneratedStoryTitleUsesTheFirstNarrativeSentence(t *testing.T) {
	tests := []struct {
		name      string
		narrative string
		want      string
	}{
		{name: "Chinese sentence", narrative: "港口的灯逐盏熄灭。远处传来汽笛声。", want: "港口的灯逐盏熄灭"},
		{name: "quoted English sentence", narrative: `“The train missed Bellweather.” Rain covered the tracks.`, want: "The train missed Bellweather"},
		{name: "first non-empty line", narrative: "\n\n雾中有人敲门\n第二段开始", want: "雾中有人敲门"},
		{name: "empty narrative", narrative: " \n\t ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generatedStoryTitle(tt.narrative); got != tt.want {
				t.Fatalf("generatedStoryTitle() = %q, want %q", got, tt.want)
			}
		})
	}

	long := strings.Repeat("雾", 40)
	if got := generatedStoryTitle(long); got != strings.Repeat("雾", 31)+"…" {
		t.Fatalf("long generated title = %q", got)
	}
}

func TestPendingStoryTitleIsGeneratedAtomicallyWithTheFirstTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if story.TitleSource != StoryTitleSourcePending {
		t.Fatalf("new unnamed story title source = %q, want pending", story.TitleSource)
	}
	if story.Title != defaultFirstStoryTitle {
		t.Fatalf("new unnamed story title = %q, want placeholder %q", story.Title, defaultFirstStoryTitle)
	}

	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		Narrative: "港口的灯逐盏熄灭。远处传来汽笛声。",
	}); err != nil {
		t.Fatal(err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Stories[0]; got.Title != "港口的灯逐盏熄灭" || got.TitleSource != StoryTitleSourceGenerated {
		t.Fatalf("generated story summary = %#v", got)
	}
	context, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if context.Meta.Title != "港口的灯逐盏熄灭" || context.Meta.TitleSource != StoryTitleSourceGenerated {
		t.Fatalf("generated story meta = %#v", context.Meta)
	}
}

func TestExplicitStoryTitleIsNeverOverwrittenByTheOpening(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "我自己的名字", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if story.TitleSource != StoryTitleSourceUser {
		t.Fatalf("explicit title source = %q, want user", story.TitleSource)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "这句话不能覆盖标题。"}); err != nil {
		t.Fatal(err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Stories[0]; got.Title != "我自己的名字" || got.TitleSource != StoryTitleSourceUser {
		t.Fatalf("explicit title changed after opening: %#v", got)
	}
}

func TestReleasedStoryWithoutTitleSourceIsNormalizedAsUserOwned(t *testing.T) {
	summary := normalizeStorySummary(StorySummary{Title: "旧故事线"})
	if summary.TitleSource != StoryTitleSourceUser {
		t.Fatalf("released summary title source = %q, want user", summary.TitleSource)
	}
	meta := normalizeStoryMeta(StoryMeta{Title: "旧故事线"})
	if meta.TitleSource != StoryTitleSourceUser {
		t.Fatalf("released meta title source = %q, want user", meta.TitleSource)
	}
}

func TestManualRenameClaimsAPendingStoryTitle(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{Title: "手动命名"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TitleSource != StoryTitleSourceUser {
		t.Fatalf("renamed title source = %q, want user", updated.TitleSource)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "另一个可能的标题。"}); err != nil {
		t.Fatal(err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if index.Stories[0].Title != "手动命名" {
		t.Fatalf("manual title was overwritten: %#v", index.Stories[0])
	}
}

func TestPendingPlaceholderIsExcludedFromTheOpeningPrompt(t *testing.T) {
	instruction, err := StoryOpeningInstruction(StoryMeta{
		Title:       defaultFirstStoryTitle,
		TitleSource: StoryTitleSourcePending,
		Opening:     StoryOpeningConfig{Mode: StoryOpeningModeAI},
		Protagonist: StoryProtagonist{Mode: StoryProtagonistModeCustom, Name: "林川"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(instruction, "Story title:") || strings.Contains(instruction, defaultFirstStoryTitle) {
		t.Fatalf("pending placeholder leaked into opening prompt: %q", instruction)
	}
}
