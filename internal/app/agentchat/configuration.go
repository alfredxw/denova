package agentchat

import (
	"context"
	"fmt"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
)

func (service *Service) ConversationConfig(ctx context.Context, binding Binding) (conversationconfig.Snapshot, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return agentconversation.PreviewSession(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
}

func (service *Service) PatchConversationConfig(
	ctx context.Context,
	binding Binding,
	patch conversationconfig.Patch,
	baseRevision uint64,
) (conversationconfig.Snapshot, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if !project.store.Exists(resolved.SessionID) {
		current, err := agentconversation.PreviewSession(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
		if err != nil {
			return conversationconfig.Snapshot{}, err
		}
		if baseRevision != 0 {
			return conversationconfig.Snapshot{}, fmt.Errorf("%w: conversation is not initialized", conversationconfig.ErrRevisionConflict)
		}
		next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
		if err != nil {
			return conversationconfig.Snapshot{}, err
		}
		sess, err := project.store.GetOrCreateWithRuntimeConfig(resolved.SessionID, next)
		if err != nil {
			return conversationconfig.Snapshot{}, err
		}
		created, ok := sess.RuntimeConfig()
		if !ok || created.Config != next || created.Revision != 1 {
			return conversationconfig.Snapshot{}, fmt.Errorf("%w: conversation was initialized concurrently", conversationconfig.ErrRevisionConflict)
		}
		return created, nil
	}
	sess, current, err := agentconversation.GetOrCreateSession(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (service *Service) conversationRuntime(ctx context.Context, binding Binding) (Binding, *projectRuntime, config.Config, error) {
	resolved, err := service.ResolveBinding(binding)
	if err != nil {
		return Binding{}, nil, config.Config{}, err
	}
	project, err := service.projectRuntime(ctx, resolved.ProjectID)
	if err != nil {
		return Binding{}, nil, config.Config{}, err
	}
	runtimeCfg, err := refreshRuntimeConfig(project)
	if err != nil {
		return Binding{}, nil, config.Config{}, err
	}
	return resolved, project, runtimeCfg, nil
}
