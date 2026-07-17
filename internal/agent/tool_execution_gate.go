package agent

import (
	"path/filepath"
	"strings"
	"sync"
)

// toolExecutionGate coordinates model-requested tool side effects for one
// workspace. Eino may invoke every tool call in an assistant message in
// parallel, so the gate keeps proven read-only tools concurrent while making
// stateful and unclassified tools exclusive.
type toolExecutionGate struct {
	mu sync.RWMutex
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

func (g *toolExecutionGate) acquire(mode toolExecutionMode) func() {
	if g == nil {
		return func() {}
	}
	if mode == toolExecutionUncoordinated {
		return func() {}
	}
	var once sync.Once
	if mode == toolExecutionParallelRead {
		g.mu.RLock()
		return func() {
			once.Do(g.mu.RUnlock)
		}
	}
	g.mu.Lock()
	return func() {
		once.Do(g.mu.Unlock)
	}
}

func executionModeForTool(manifest ToolManifest) toolExecutionMode {
	if manifest.Name == "task" {
		// task is an orchestration boundary. Holding the workspace lock while
		// its subagent runs would deadlock when that subagent invokes a gated
		// file tool; the nested tools acquire their own shared-workspace leases.
		return toolExecutionUncoordinated
	}
	if manifest.MutatesWorkspace || manifest.Source == ToolSourceShell {
		return toolExecutionExclusive
	}
	switch manifest.Source {
	case ToolSourceRead, ToolSourceLore, ToolSourceHistory, ToolSourceWeb:
		return toolExecutionParallelRead
	default:
		// Only tools classified by a stable manifest are allowed to share the
		// read side. Unknown tools remain exclusive because their side effects
		// cannot be proven safe.
		return toolExecutionExclusive
	}
}
