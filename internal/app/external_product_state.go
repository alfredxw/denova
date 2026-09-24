package app

import (
	"denova/config"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
)

func (a *App) agentSession(options agentrun.Options, native *agentexecution.Runtime) (*agentruntime.Session, error) {
	if options.AgentKind != agentrun.AgentKindInteractiveStory {
		a.mu.RLock()
		sessions := a.sessionStore
		a.mu.RUnlock()
		journal, err := sessions.Get(options.SessionID)
		if err != nil {
			return nil, err
		}
		snapshot, _ := journal.RuntimeConfig()
		return a.AgentEngines().ConversationSession(native, options, journal, snapshot.Engine())
	}
	state, selected, err := a.externalProductState(options)
	if err != nil {
		return nil, err
	}
	if !selected {
		return agentruntime.NativeSession(native, options), nil
	}
	return a.AgentEngines().ExternalSession(options, state, nil)
}

// externalProductState selects the product state adapter independently of the
// Native execution service. Reading or mutating an external Goal opens no Agent.
func (a *App) externalProductState(options agentrun.Options) (agentruntime.ProductState, bool, error) {
	a.mu.RLock()
	sessions, stories := a.sessionStore, a.interactive
	a.mu.RUnlock()
	if options.AgentKind == agentrun.AgentKindInteractiveStory {
		snapshot, found, err := stories.BranchRuntimeConfig(options.StoryID, options.BranchID)
		if err != nil || !found || snapshot.Engine().Kind == config.RuntimeNative {
			return agentruntime.ProductState{}, false, err
		}
		state, err := agentruntime.GameState(options, stories)
		return state, true, err
	}
	sess, err := sessions.Get(options.SessionID)
	if err != nil {
		return agentruntime.ProductState{}, false, err
	}
	snapshot, found := sess.RuntimeConfig()
	if !found || snapshot.Engine().Kind == config.RuntimeNative {
		return agentruntime.ProductState{}, false, nil
	}
	state, err := agentruntime.SessionState(options, sess)
	return state, true, err
}

func (a *App) externalController(options agentrun.Options) (*agentruntime.ExternalController, bool, error) {
	state, selected, err := a.externalProductState(options)
	if err != nil || !selected {
		return nil, selected, err
	}
	control, err := a.AgentEngines().ExternalControl(options, state)
	return control, true, err
}
