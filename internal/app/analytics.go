package app

import (
	"log/slog"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/analytics"
)

// StartAnalytics installs the optional host service at process startup. A
// failure disables statistics and never prevents loading creative data.
func (a *App) StartAnalytics(version string) {
	service, err := analytics.Open(analytics.Options{Version: version, Development: a.cfg.DevMode})
	if err != nil {
		slog.Warn("analytics_initialization_failed", "error", err)
		return
	}
	a.mu.Lock()
	a.analytics = service
	a.mu.Unlock()
}

func (a *App) Analytics() *analytics.Service {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.analytics
}

func analyticsEngine(cfg config.Config) analytics.Engine {
	if cfg.ActiveAgentRuntime == nil {
		return analytics.Native
	}
	switch cfg.ActiveAgentRuntime.Kind {
	case "", config.RuntimeNative:
		return analytics.Native
	case config.RuntimeCodex:
		return analytics.Codex
	case config.RuntimeClaude:
		return analytics.Claude
	case config.RuntimeGrok:
		return analytics.Grok
	case config.RuntimeAntigravity:
		return analytics.Antigravity
	}
	slog.Error("analytics_runtime_unknown", "runtime", cfg.ActiveAgentRuntime.Kind)
	return ""
}

func finishAnalyticsRun(run analytics.Run, status agentrun.OutcomeStatus) {
	switch status {
	case agentrun.OutcomeCompleted:
		run.Finish(analytics.Success)
	case agentrun.OutcomeFailed:
		run.Finish(analytics.Failure)
	case agentrun.OutcomeAborted:
		run.Finish(analytics.Cancelled)
	case agentrun.OutcomePreempted:
		run.Finish(analytics.Preempted)
	case agentrun.OutcomeSuspended:
		run.Finish(analytics.Suspended)
	default:
		slog.Error("analytics_outcome_unknown", "status", status)
	}
}
