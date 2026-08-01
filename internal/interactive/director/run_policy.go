package director

import (
	"fmt"
	"strings"
)

const (
	RunModeOnDemand = "on_demand"
	RunModeManual   = "manual"
	RunModeInterval = "interval"

	DefaultIntervalTurns = 3

	DefaultAgentMode   = AgentModeTriggered
	AgentModeTriggered = "triggered"
	AgentModeEveryTurn = "every_turn"
	AgentModeOff       = "off"

	PlanStatusWaitingOpening = "waiting_opening"
	PlanStatusRunning        = "running"
	PlanStatusReady          = "ready"
	PlanStatusSkipped        = "skipped"
	PlanStatusFailed         = "failed"
	PlanStatusConflict       = "conflict"
)

// RunPolicy controls background Director scheduling for one
// story. IntervalTurns is used only by interval mode; the first committed turn
// always initializes the plan before the interval cadence begins.
type RunPolicy struct {
	Mode          string `json:"mode"`
	IntervalTurns int    `json:"interval_turns,omitempty"`
}

// ScheduleContext is the committed branch state observed after a
// Game Agent turn has been persisted.
type ScheduleContext struct {
	CommittedTurns int
	PlanStatus     string
	MaterialUpdate bool
}

// ScheduleDecision explains whether the Director should run after a commit.
type ScheduleDecision struct {
	ShouldRun bool   `json:"should_run"`
	Reason    string `json:"reason,omitempty"`
}

func NormalizeRunPolicy(policy RunPolicy) RunPolicy {
	policy.Mode = strings.TrimSpace(policy.Mode)
	if policy.Mode == "" {
		policy.Mode = RunModeOnDemand
	}
	if policy.Mode == RunModeInterval {
		if policy.IntervalTurns == 0 {
			policy.IntervalTurns = DefaultIntervalTurns
		}
	} else {
		policy.IntervalTurns = 0
	}
	return policy
}

func ValidateRunPolicy(policy RunPolicy) error {
	policy = NormalizeRunPolicy(policy)
	switch policy.Mode {
	case RunModeOnDemand, RunModeManual:
		return nil
	case RunModeInterval:
		if policy.IntervalTurns <= 0 {
			return fmt.Errorf("后台导演自动运行间隔必须大于 0 / Director auto-run interval must be greater than 0")
		}
		return nil
	default:
		return fmt.Errorf("后台导演运行模式无效 / Invalid Director run mode: %q", policy.Mode)
	}
}

// LegacyRunPolicy maps the former reusable-preset setting to a
// story policy for clients and stories that do not yet persist one.
func LegacyRunPolicy(agentMode string) RunPolicy {
	switch strings.TrimSpace(agentMode) {
	case AgentModeEveryTurn:
		return RunPolicy{Mode: RunModeInterval, IntervalTurns: 1}
	case AgentModeOff:
		return RunPolicy{Mode: RunModeManual}
	case AgentModeTriggered:
		return RunPolicy{Mode: RunModeOnDemand}
	default:
		return RunPolicy{Mode: RunModeOnDemand}
	}
}

// ResolveRunPolicy prefers the story-scoped policy and falls back
// to the selected Director preset only for legacy stories and clients.
func ResolveRunPolicy(policy *RunPolicy, agentMode string) RunPolicy {
	if policy == nil {
		return LegacyRunPolicy(agentMode)
	}
	return NormalizeRunPolicy(*policy)
}

// DecideDirectorRunAfterTurn evaluates one story's scheduling policy after a
// durable turn. Manual mode never schedules work, but does not prevent the
// explicit manual-run interface from starting the Director.
func DecideRunAfterTurn(enabled bool, policy RunPolicy, context ScheduleContext) ScheduleDecision {
	if !enabled {
		return ScheduleDecision{Reason: "disabled"}
	}
	policy = NormalizeRunPolicy(policy)
	if err := ValidateRunPolicy(policy); err != nil {
		return ScheduleDecision{Reason: "invalid_policy"}
	}
	switch policy.Mode {
	case RunModeManual:
		return ScheduleDecision{Reason: "manual_mode"}
	case RunModeOnDemand:
		if context.PlanStatus == PlanStatusWaitingOpening {
			return ScheduleDecision{ShouldRun: true, Reason: "initial_plan"}
		}
		if context.MaterialUpdate {
			return ScheduleDecision{ShouldRun: true, Reason: "game_agent_update"}
		}
		return ScheduleDecision{Reason: "no_material_update"}
	case RunModeInterval:
		if context.PlanStatus == PlanStatusWaitingOpening {
			return ScheduleDecision{ShouldRun: true, Reason: "initial_plan"}
		}
		if context.CommittedTurns > 0 && (context.CommittedTurns-1)%policy.IntervalTurns == 0 {
			return ScheduleDecision{ShouldRun: true, Reason: "interval_turn"}
		}
		return ScheduleDecision{Reason: "interval_wait"}
	default:
		return ScheduleDecision{Reason: "invalid_policy"}
	}
}
