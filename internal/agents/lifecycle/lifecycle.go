// Package lifecycle adapts Denova's application boundary to the public
// Agent -> Session -> Run lifecycle.
package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
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
type EffectRequest = agent.EffectRequest
type EffectResult = agent.EffectResult

// Config declares Denova-owned transcript storage and optional integrations.
type Config struct {
	StoreRoot         string
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
	root := strings.TrimSpace(config.StoreRoot)
	if root == "" {
		return nil, errors.New("Denova Agent lifecycle StoreRoot is required")
	}
	if source == nil {
		return nil, errors.New("Denova Agent lifecycle Source is required")
	}
	store, err := sessionfile.New(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	runIDs := config.RunIDGenerator
	if runIDs == nil {
		runIDs = DefaultRunIDGenerator
	}
	options := []agent.Option{
		agent.WithSessionStore(store), agent.WithRunIDGenerator(runIDs),
	}
	if config.CacheKeyGenerator != nil {
		options = append(options, agent.WithCacheKeyGenerator(config.CacheKeyGenerator))
	}
	if config.Trace != nil {
		options = append(options, agent.WithTrace(config.Trace))
	}
	return agent.New(ctx, source, options...)
}
