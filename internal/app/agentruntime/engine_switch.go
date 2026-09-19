package agentruntime

import (
	"context"
	"errors"
	"reflect"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// AdmitExecution guards the final accept step against an engine change made
// while product context was being prepared. Release immediately after Start,
// never hold this lock during model execution or an Ask wait.
func (engines *Engines) AdmitExecution(ctx context.Context, sess *session.Session, prepared *config.RuntimeSelection) (func(), error) {
	engines.admission.RLock()
	want := config.RuntimeSelection{Kind: config.RuntimeNative}
	if prepared != nil {
		want = *prepared
	}
	err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		if !reflect.DeepEqual(state.Config.Engine(), want) {
			return conversationconfig.ErrRevisionConflict
		}
		return nil
	})
	if err != nil {
		engines.admission.RUnlock()
		return nil, err
	}
	return engines.admission.RUnlock, nil
}

// ApplyEngineSelection validates and commits under shared admission. Saving
// Agent defaults deliberately neither calls this method nor probes an engine.
func (engines *Engines) ApplyEngineSelection(ctx context.Context, native *execution.Runtime, sess *session.Session, options agentrun.Options, next conversationconfig.Config, revision uint64, cfg config.Config) (conversationconfig.Snapshot, error) {
	engines.admission.Lock()
	defer engines.admission.Unlock()
	if err := engines.validateEngineSwitch(ctx, native, sess, options, next.Engine(), cfg); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, revision)
}

func (engines *Engines) validateEngineSwitch(ctx context.Context, native *execution.Runtime, sess *session.Session, options agentrun.Options, target config.RuntimeSelection, cfg config.Config) error {
	if err := engines.Operations.Recover(ctx, options.ProjectID, sess); err != nil {
		return err
	}
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error { return state.Projection.RequireIdle() }); err != nil {
		return err
	}
	modes := []string{options.Mode}
	if options.AgentKind == config.AgentKindIDE {
		modes = []string{"ide", "agent_chat"}
	}
	for _, mode := range modes {
		candidate := options
		candidate.Mode = mode
		err := native.RequireIdleForEngineSwitch(ctx, candidate)
		if errors.Is(err, agent.ErrSessionBusy) {
			return ErrOperationActive
		}
		if err != nil && !errors.Is(err, execution.ErrRuntimeProjectionUnavailable) {
			return err
		}
	}
	if target.Kind == config.RuntimeNative {
		return nil
	}
	_, release, err := engines.Acquire(ctx, target, cfg)
	if err == nil {
		release()
	}
	return err
}
