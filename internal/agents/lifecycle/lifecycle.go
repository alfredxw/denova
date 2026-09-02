// Package lifecycle adapts Denova's application boundary to the public
// Agent -> Session -> Run lifecycle.
package lifecycle

import (
	"context"
	"errors"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// Lifecycle-facing aliases keep application packages on Denova's adapter
// boundary while preserving the public lifecycle API unchanged.
type Agent = agent.Agent
type Session = agent.Session
type Run = agent.Run
type Input = agent.Input
type Event = agent.Event
type Result = agent.Result
type Snapshot = agent.SessionSnapshot
type Observation = agent.Observation
type InteractionResponse = agent.InteractionResponse
type SessionKey = agent.SessionKey
type SessionSelector = agent.SessionSelector

type CanonicalAdapter = agent.CanonicalAdapter
type CanonicalAdapterFuncs = agent.CanonicalAdapterFuncs
type InputCommitRequest = agent.InputCommitRequest
type OutputCommitRequest = agent.OutputCommitRequest
type CommitReceipt = agent.CommitReceipt
type OutputCommitReceipt = agent.OutputCommitReceipt
type OutputProjection = agent.OutputProjection
type ContextCommitRequest = agent.ContextCommitRequest
type EffectRequest = agent.EffectRequest
type EffectResult = agent.EffectResult

// Config declares Denova-owned Session storage and optional integrations.
type Config struct {
	Store             agentsession.Store
	Trace             agent.TraceSink
	RunIDGenerator    agent.RunIDGenerator
	CacheKeyGenerator agent.CacheKeyGenerator
}

// DefaultRunIDGenerator is Denova's application-owned execution identity
// policy. Agent treats the returned value as opaque.
func DefaultRunIDGenerator(agent.RunIDRequest) (string, error) {
	return agentrun.NewID("run"), nil
}

func New(ctx context.Context, source agent.Source, config Config) (*Agent, error) {
	if config.Store == nil {
		return nil, errors.New("Denova Agent lifecycle Store is required")
	}
	if source == nil {
		return nil, errors.New("Denova Agent lifecycle Source is required")
	}
	runIDs := config.RunIDGenerator
	if runIDs == nil {
		runIDs = DefaultRunIDGenerator
	}
	options := []agent.Option{
		agent.WithSessionStore(config.Store), agent.WithRunIDGenerator(runIDs),
	}
	if config.CacheKeyGenerator != nil {
		options = append(options, agent.WithCacheKeyGenerator(config.CacheKeyGenerator))
	}
	if config.Trace != nil {
		options = append(options, agent.WithTrace(config.Trace))
	}
	return agent.New(ctx, source, options...)
}
