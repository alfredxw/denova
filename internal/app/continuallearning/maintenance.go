package continuallearning

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

const ScheduledSessionID = "harness-scheduled"

// StartTask delegates scheduled maintenance to the same project-scoped
// AgentChat runtime used by interactive Harness conversations.
func (service *Service) StartTask(ctx context.Context, request Request) (*apptask.Task, error) {
	if _, err := service.requireEnabled(); err != nil {
		return nil, err
	}
	request.CommandID = strings.TrimSpace(request.CommandID)
	if request.CommandID == "" {
		return nil, apptask.ErrCommandIDRequired
	}
	if err := agentrun.ValidateCommandID(request.CommandID); err != nil {
		return nil, err
	}
	request.Trigger = normalizeTrigger(request.Trigger)
	evidence, err := normalizeTrajectoryEvidence(request.Evidence)
	if err != nil {
		return nil, err
	}
	request.Evidence = evidence
	if strings.TrimSpace(request.Instruction) == "" {
		request.Instruction = "Review recent trajectory evidence since prior maintenance. Make only evidence-backed, reusable Harness State improvements; use a no-op when no change is justified."
	}
	return service.host.StartHarnessTurn(ctx, HarnessTurnRequest{
		CommandID: request.CommandID,
		SessionID: ScheduledSessionID,
		Message:   maintenanceMessage(request),
		Locale:    request.Locale,
	})
}

func normalizeTrigger(value string) string {
	if strings.TrimSpace(value) == TriggerScheduled {
		return TriggerScheduled
	}
	return TriggerManual
}

func maintenanceMessage(request Request) string {
	evidenceScope := "Discover relevant recent evidence through trajectory://index."
	if request.Evidence != nil {
		if len(request.Evidence) == 0 {
			evidenceScope = "No trajectory was selected. Do not broaden the analysis unless explicitly requested."
		} else {
			evidenceScope = "Use only these selected trajectory resources as the analysis basis:\n- " + strings.Join(request.Evidence, "\n- ")
		}
	}
	return strings.TrimSpace(fmt.Sprintf(`[Maintenance Trigger]
- trigger: %s
- trajectory_index: trajectory://index
- validation_status: harness://state/current

[Analysis Evidence]
%s

[Task]
Evaluate recurring failures and durable user preferences. Inspect the relevant trajectory resources and current Harness State. When a minimal reusable improvement is justified, edit the live Project workspace with ordinary file tools. Then inspect harness://state/current and repair every diagnostic. Never copy project content, complete trajectories, secrets, or private reasoning into State.

[User Instruction]
%s`, normalizeTrigger(request.Trigger), evidenceScope, strings.TrimSpace(request.Instruction)))
}

func normalizeTrajectoryEvidence(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) > 500 {
		return nil, fmt.Errorf("trajectory evidence cannot exceed 500 resources")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("invalid trajectory evidence resource")
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed == nil {
			return nil, fmt.Errorf("invalid trajectory evidence resource %q", value)
		}
		segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		if parsed.Scheme != "trajectory" || parsed.Host != "projects" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(segments) != 3 || (segments[1] != "sessions" && segments[1] != "runs") {
			return nil, fmt.Errorf("invalid trajectory evidence resource %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
