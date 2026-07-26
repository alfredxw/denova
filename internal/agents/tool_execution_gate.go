package agents

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

const toolExecutionGateCapacity int64 = 1<<63 - 1

// toolExecutionGate coordinates model-requested tool side effects for one
// workspace. The native loop may invoke every tool call in an assistant message
// in parallel, so the gate keeps proven read-only tools concurrent while making
// stateful and unclassified tools exclusive.
type toolExecutionGate struct {
	init      sync.Once
	admission *semaphore.Weighted
}

type toolExecutionMode uint8

const (
	toolExecutionExclusive toolExecutionMode = iota
	toolExecutionParallelRead
	toolExecutionUncoordinated
)

var sharedToolExecutionGates sync.Map

func sharedToolExecutionGate(workspace string) *toolExecutionGate {
	key := strings.TrimSpace(workspace)
	if key == "" {
		return &toolExecutionGate{}
	}
	if absolute, err := filepath.Abs(key); err == nil {
		key = absolute
	}
	if canonical, err := filepath.EvalSymlinks(key); err == nil {
		key = canonical
	}
	key = filepath.Clean(key)
	gate, _ := sharedToolExecutionGates.LoadOrStore(key, &toolExecutionGate{})
	return gate.(*toolExecutionGate)
}

func (g *toolExecutionGate) acquire(ctx context.Context, mode toolExecutionMode) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g == nil || mode == toolExecutionUncoordinated {
		return func() {}, nil
	}

	weight := toolExecutionGateCapacity
	if mode == toolExecutionParallelRead {
		weight = 1
	}
	admission := g.weightedAdmission()
	if err := admission.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		admission.Release(weight)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			admission.Release(weight)
		})
	}, nil
}

func (g *toolExecutionGate) weightedAdmission() *semaphore.Weighted {
	g.init.Do(func() {
		g.admission = semaphore.NewWeighted(toolExecutionGateCapacity)
	})
	return g.admission
}

func executionModeForTool(manifest ToolManifest) toolExecutionMode {
	switch manifest.Execution {
	case ToolExecutionChild, ToolExecutionInteractiveWait:
		// task is an orchestration boundary. Holding the workspace lock while
		// its subagent runs would deadlock when that subagent invokes a gated
		// file tool; the nested tools acquire their own shared-workspace leases.
		return toolExecutionUncoordinated
	case ToolExecutionParallelRead:
		return toolExecutionParallelRead
	case ToolExecutionWorkspaceExclusive, ToolExecutionSessionExclusive, ToolExecutionConfigExclusive:
		return toolExecutionExclusive
	default:
		// Only tools classified by a stable manifest are allowed to share the
		// read side. Unknown tools remain exclusive because their side effects
		// cannot be proven safe.
		return toolExecutionExclusive
	}
}
