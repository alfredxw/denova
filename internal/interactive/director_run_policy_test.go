package interactive

import "testing"

func TestDecideDirectorRunAfterTurn(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		policy     StoryDirectorRunPolicy
		context    DirectorRunScheduleContext
		wantRun    bool
		wantReason string
	}{
		{
			name:       "on demand initializes after opening",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeOnDemand},
			context:    DirectorRunScheduleContext{CommittedTurns: 1, PlanStatus: DirectorPlanStatusWaitingOpening},
			wantRun:    true,
			wantReason: "initial_plan",
		},
		{
			name:       "on demand skips routine turn after initialization",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeOnDemand},
			context:    DirectorRunScheduleContext{CommittedTurns: 2, PlanStatus: DirectorPlanStatusReady},
			wantReason: "no_material_update",
		},
		{
			name:       "on demand follows material game signal",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeOnDemand},
			context:    DirectorRunScheduleContext{CommittedTurns: 2, PlanStatus: DirectorPlanStatusReady, MaterialUpdate: true},
			wantRun:    true,
			wantReason: "game_agent_update",
		},
		{
			name:       "manual never starts automatically",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeManual},
			context:    DirectorRunScheduleContext{CommittedTurns: 1, PlanStatus: DirectorPlanStatusWaitingOpening, MaterialUpdate: true},
			wantReason: "manual_mode",
		},
		{
			name:       "interval initializes after opening",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeInterval, IntervalTurns: 3},
			context:    DirectorRunScheduleContext{CommittedTurns: 1, PlanStatus: DirectorPlanStatusWaitingOpening},
			wantRun:    true,
			wantReason: "initial_plan",
		},
		{
			name:       "interval waits between cadence turns",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeInterval, IntervalTurns: 3},
			context:    DirectorRunScheduleContext{CommittedTurns: 3, PlanStatus: DirectorPlanStatusReady},
			wantReason: "interval_wait",
		},
		{
			name:       "interval runs every configured turns after initialization",
			enabled:    true,
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeInterval, IntervalTurns: 3},
			context:    DirectorRunScheduleContext{CommittedTurns: 4, PlanStatus: DirectorPlanStatusReady},
			wantRun:    true,
			wantReason: "interval_turn",
		},
		{
			name:       "disabled director overrides story policy",
			policy:     StoryDirectorRunPolicy{Mode: DirectorRunModeInterval, IntervalTurns: 1},
			context:    DirectorRunScheduleContext{CommittedTurns: 1, PlanStatus: DirectorPlanStatusWaitingOpening},
			wantReason: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideDirectorRunAfterTurn(tt.enabled, tt.policy, tt.context)
			if decision.ShouldRun != tt.wantRun || decision.Reason != tt.wantReason {
				t.Fatalf("unexpected schedule decision: got %#v, want run=%t reason=%q", decision, tt.wantRun, tt.wantReason)
			}
		})
	}
}

func TestStorePersistsStoryDirectorRunPolicy(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "定时导演",
		DirectorRunPolicy: &StoryDirectorRunPolicy{
			Mode:          DirectorRunModeInterval,
			IntervalTurns: 4,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if story.DirectorRunPolicy == nil || story.DirectorRunPolicy.Mode != DirectorRunModeInterval || story.DirectorRunPolicy.IntervalTurns != 4 {
		t.Fatalf("create result should expose the persisted policy: %#v", story.DirectorRunPolicy)
	}
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if storyContext.Meta.DirectorRunPolicy == nil || *storyContext.Meta.DirectorRunPolicy != *story.DirectorRunPolicy {
		t.Fatalf("story metadata should preserve the policy: %#v", storyContext.Meta.DirectorRunPolicy)
	}

	manual := StoryDirectorRunPolicy{Mode: DirectorRunModeManual}
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{DirectorRunPolicy: &manual})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DirectorRunPolicy == nil || updated.DirectorRunPolicy.Mode != DirectorRunModeManual || updated.DirectorRunPolicy.IntervalTurns != 0 {
		t.Fatalf("story update should replace the policy: %#v", updated.DirectorRunPolicy)
	}
}
