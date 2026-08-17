package director

import "testing"

func TestDecideDirectorRunAfterTurn(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		policy     RunPolicy
		context    ScheduleContext
		wantRun    bool
		wantReason string
	}{
		{
			name:       "on demand initializes after opening",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeOnDemand},
			context:    ScheduleContext{CommittedTurns: 1, PlanStatus: PlanStatusWaitingOpening},
			wantRun:    true,
			wantReason: "initial_plan",
		},
		{
			name:       "on demand skips routine turn after initialization",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeOnDemand},
			context:    ScheduleContext{CommittedTurns: 2, PlanStatus: PlanStatusReady},
			wantReason: "no_material_update",
		},
		{
			name:       "on demand follows material game signal",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeOnDemand},
			context:    ScheduleContext{CommittedTurns: 2, PlanStatus: PlanStatusReady, MaterialUpdate: true},
			wantRun:    true,
			wantReason: "game_agent_update",
		},
		{
			name:       "manual never starts automatically",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeManual},
			context:    ScheduleContext{CommittedTurns: 1, PlanStatus: PlanStatusWaitingOpening, MaterialUpdate: true},
			wantReason: "manual_mode",
		},
		{
			name:       "interval initializes after opening",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeInterval, IntervalTurns: 3},
			context:    ScheduleContext{CommittedTurns: 1, PlanStatus: PlanStatusWaitingOpening},
			wantRun:    true,
			wantReason: "initial_plan",
		},
		{
			name:       "interval waits between cadence turns",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeInterval, IntervalTurns: 3},
			context:    ScheduleContext{CommittedTurns: 3, PlanStatus: PlanStatusReady},
			wantReason: "interval_wait",
		},
		{
			name:       "interval runs every configured turns after initialization",
			enabled:    true,
			policy:     RunPolicy{Mode: RunModeInterval, IntervalTurns: 3},
			context:    ScheduleContext{CommittedTurns: 4, PlanStatus: PlanStatusReady},
			wantRun:    true,
			wantReason: "interval_turn",
		},
		{
			name:       "disabled director overrides story policy",
			policy:     RunPolicy{Mode: RunModeInterval, IntervalTurns: 1},
			context:    ScheduleContext{CommittedTurns: 1, PlanStatus: PlanStatusWaitingOpening},
			wantReason: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideRunAfterTurn(tt.enabled, tt.policy, tt.context)
			if decision.ShouldRun != tt.wantRun || decision.Reason != tt.wantReason {
				t.Fatalf("unexpected schedule decision: got %#v, want run=%t reason=%q", decision, tt.wantRun, tt.wantReason)
			}
		})
	}
}
