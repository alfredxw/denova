package agentruntime

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"denova/config"
	"denova/internal/agents/external"
	"denova/internal/agents/external/claude"
	"denova/internal/agents/external/codex"
	"denova/internal/hostruntime"
)

func connectRuntimeAPI(ctx context.Context, kind config.RuntimeID, model config.ResolvedModelSettings) (external.Connection, error) {
	switch kind {
	case config.RuntimeCodex:
		executable := hostruntime.DiscoverCodex(os.Environ())
		if executable == "" {
			return nil, ErrEngineNotInstalled
		}
		home, err := hostruntime.CodexHome(os.Environ())
		if err != nil {
			return nil, err
		}
		return codex.Connect(ctx, codex.ProcessOptions{Executable: executable, Home: home, API: &model})
	case config.RuntimeClaude:
		launch := hostruntime.DiscoverClaude(os.Environ())
		if launch.Executable == "" {
			return nil, ErrEngineNotInstalled
		}
		return claude.Connect(ctx, claude.ProcessOptions{Launch: launch, API: &model})
	default:
		return nil, ErrEngineNotFound
	}
}

// acquireAPI owns a fresh process per operation, including its maintenance
// calls. Concurrent profiles and credential updates cannot alter this snapshot.
func (engines *Engines) acquireAPI(ctx context.Context, selection config.RuntimeSelection, cfg config.Config) (external.Adapter, func(), error) {
	model, err := config.ResolveRuntimeModel(&cfg, selection)
	if err != nil {
		return nil, nil, err
	}
	engines.apiMu.Lock()
	defer engines.apiMu.Unlock()
	if engines.apiClosed {
		return nil, nil, ErrEngineNotReady
	}
	connection, err := engines.apiFactory(ctx, selection.Kind, model)
	if err != nil {
		return nil, nil, err
	}
	// Model catalogs and CLI sign-in are irrelevant to explicit API routing.
	engines.apiConnections[connection] = struct{}{}
	slog.InfoContext(ctx, "[external-runtime] acquired API connection", "runtime", selection.Kind, "profile", model.ProfileID, "model", model.Model)
	var once sync.Once
	release := func() {
		once.Do(func() {
			engines.apiMu.Lock()
			defer engines.apiMu.Unlock()
			if _, active := engines.apiConnections[connection]; active {
				if err := connection.Close(); err != nil {
					slog.Warn("[external-runtime] close API connection failed", "runtime", selection.Kind)
				}
				delete(engines.apiConnections, connection)
			}
		})
	}
	return connection, release, nil
}
