package execution

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// ChildDefinitionRequest rebuilds one named child from the exact durable
// parent Session. HostData is the immutable accepted parent-turn descriptor;
// delegated Sessions still own isolated transcripts and canonical state.
type ChildDefinitionRequest struct {
	Parent   agent.SessionKey
	Child    string
	HostData *agent.HostData
}

// ChildDefinitionResolver rebuilds composition from stable product identity
// and current configuration, so cold task recovery never depends on an
// executor-local task map.
type ChildDefinitionResolver interface {
	PrepareChildDefinition(context.Context, ChildDefinitionRequest) (agent.Definition, error)
}

type ChildDefinitionResolverFunc func(context.Context, ChildDefinitionRequest) (agent.Definition, error)

func (resolve ChildDefinitionResolverFunc) PrepareChildDefinition(
	ctx context.Context,
	request ChildDefinitionRequest,
) (agent.Definition, error) {
	if resolve == nil || strings.TrimSpace(request.Child) == "" {
		return agent.Definition{}, errors.New("delegated Agent Definition resolver is unavailable")
	}
	return resolve(ctx, request)
}
