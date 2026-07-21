package interactive

import (
	"fmt"
	"strings"
)

const (
	DirectorRunModeOnDemand = "on_demand"
	DirectorRunModeManual   = "manual"
	DirectorRunModeInterval = "interval"

	DefaultStoryDirectorIntervalTurns = 3
)

// StoryDirectorRunPolicy controls background Director scheduling for one
// story. IntervalTurns is used only by interval mode; the first committed turn
// always initializes the plan before the interval cadence begins.
type StoryDirectorRunPolicy struct {
	Mode          string `json:"mode"`
	IntervalTurns int    `json:"interval_turns,omitempty"`
}

// DirectorRunScheduleContext is the committed branch state observed after a
// Game Agent turn has been persisted.
type DirectorRunScheduleContext struct {
	CommittedTurns int
	PlanStatus     string
	MaterialUpdate bool
}

func NormalizeStoryDirectorRunPolicy(policy StoryDirectorRunPolicy) StoryDirectorRunPolicy {
	policy.Mode = strings.TrimSpace(policy.Mode)
	if policy.Mode == "" {
		policy.Mode = DirectorRunModeOnDemand
	}
	if policy.Mode == DirectorRunModeInterval {
		if policy.IntervalTurns == 0 {
			policy.IntervalTurns = DefaultStoryDirectorIntervalTurns
		}
	} else {
		policy.IntervalTurns = 0
	}
	return policy
}

func ValidateStoryDirectorRunPolicy(policy StoryDirectorRunPolicy) error {
	policy = NormalizeStoryDirectorRunPolicy(policy)
	switch policy.Mode {
	case DirectorRunModeOnDemand, DirectorRunModeManual:
		return nil
	case DirectorRunModeInterval:
		if policy.IntervalTurns <= 0 {
			return fmt.Errorf("后台导演自动运行间隔必须大于 0 / Director auto-run interval must be greater than 0")
		}
		return nil
	default:
		return fmt.Errorf("后台导演运行模式无效 / Invalid Director run mode: %q", policy.Mode)
	}
}

// LegacyStoryDirectorRunPolicy maps the former reusable-preset setting to a
// story policy for clients and stories that do not yet persist one.
func LegacyStoryDirectorRunPolicy(strategy StoryDirectorStrategy) StoryDirectorRunPolicy {
	strategy = NormalizeStoryDirectorStrategy(strategy)
	switch strategy.DirectorAgentMode {
	case DirectorAgentModeEveryTurn:
		return StoryDirectorRunPolicy{Mode: DirectorRunModeInterval, IntervalTurns: 1}
	case DirectorAgentModeOff:
		return StoryDirectorRunPolicy{Mode: DirectorRunModeManual}
	case DirectorAgentModeTriggered:
		return StoryDirectorRunPolicy{Mode: DirectorRunModeOnDemand}
	default:
		return StoryDirectorRunPolicy{Mode: DirectorRunModeOnDemand}
	}
}

// ResolveStoryDirectorRunPolicy prefers the story-scoped policy and falls back
// to the selected Director preset only for legacy stories and clients.
func ResolveStoryDirectorRunPolicy(policy *StoryDirectorRunPolicy, strategy StoryDirectorStrategy) StoryDirectorRunPolicy {
	if policy == nil {
		return LegacyStoryDirectorRunPolicy(strategy)
	}
	return NormalizeStoryDirectorRunPolicy(*policy)
}

// DecideDirectorRunAfterTurn evaluates one story's scheduling policy after a
// durable turn. Manual mode never schedules work, but does not prevent the
// explicit manual-run interface from starting the Director.
func DecideDirectorRunAfterTurn(enabled bool, policy StoryDirectorRunPolicy, context DirectorRunScheduleContext) DirectorAgentScheduleDecision {
	if !enabled {
		return DirectorAgentScheduleDecision{Reason: "disabled"}
	}
	policy = NormalizeStoryDirectorRunPolicy(policy)
	if err := ValidateStoryDirectorRunPolicy(policy); err != nil {
		return DirectorAgentScheduleDecision{Reason: "invalid_policy"}
	}
	switch policy.Mode {
	case DirectorRunModeManual:
		return DirectorAgentScheduleDecision{Reason: "manual_mode"}
	case DirectorRunModeOnDemand:
		if context.PlanStatus == DirectorPlanStatusWaitingOpening {
			return DirectorAgentScheduleDecision{ShouldRun: true, Reason: "initial_plan"}
		}
		if context.MaterialUpdate {
			return DirectorAgentScheduleDecision{ShouldRun: true, Reason: "game_agent_update"}
		}
		return DirectorAgentScheduleDecision{Reason: "no_material_update"}
	case DirectorRunModeInterval:
		if context.PlanStatus == DirectorPlanStatusWaitingOpening {
			return DirectorAgentScheduleDecision{ShouldRun: true, Reason: "initial_plan"}
		}
		if context.CommittedTurns > 0 && (context.CommittedTurns-1)%policy.IntervalTurns == 0 {
			return DirectorAgentScheduleDecision{ShouldRun: true, Reason: "interval_turn"}
		}
		return DirectorAgentScheduleDecision{Reason: "interval_wait"}
	default:
		return DirectorAgentScheduleDecision{Reason: "invalid_policy"}
	}
}
