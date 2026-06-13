package agent

import (
	"context"

	"github.com/cloudwego/eino/adk"
)

// NewRunner 创建支持流式输出的 Agent Runner。
func NewRunner(ctx context.Context, builtAgent adk.Agent) *adk.Runner {
	return NewRunnerWithOptions(ctx, builtAgent, RunOptions{})
}

// NewRunnerWithOptions creates a runner with workspace-scoped checkpoints when
// a workspace is available.
func NewRunnerWithOptions(ctx context.Context, builtAgent adk.Agent, options RunOptions) *adk.Runner {
	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           builtAgent,
		EnableStreaming: true,
		CheckPointStore: newCheckpointStore(options.Workspace, options.AgentKind),
	})
}

// inMemoryStore 简单的内存 CheckPoint 存储。
type inMemoryStore struct {
	mem map[string][]byte
}

func (s *inMemoryStore) Set(_ context.Context, key string, value []byte) error {
	s.mem[key] = value
	return nil
}

func (s *inMemoryStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := s.mem[key]
	return v, ok, nil
}
