package imageapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	imagegen "denova/internal/image/generation"
)

// DiscoverComfyUIWorkflows inspects an unsaved ComfyUI profile without
// requiring a model or workflow to have been selected yet.
func (service *Service) DiscoverComfyUIWorkflows(ctx context.Context, endpoint config.ImageAPIEndpointSettings, profile config.ImageAPIProfileSettings) (imagegen.ComfyUIWorkflowCatalog, error) {
	resolved, err := service.resolveComfyUIConnection(endpoint, profile)
	if err != nil {
		return imagegen.ComfyUIWorkflowCatalog{}, fmt.Errorf("discover ComfyUI workflows: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[image-comfyui] discovering saved workflows profile_id=%s base_url=%s", resolved.ProfileID, resolved.BaseURL))
	catalog, err := imagegen.NewComfyUIAdapter(nil).DiscoverWorkflows(ctx, resolved)
	if err != nil {
		return imagegen.ComfyUIWorkflowCatalog{}, &ProviderRequestError{cause: fmt.Errorf("discover ComfyUI workflows: %w", err)}
	}
	slog.InfoContext(ctx, fmt.Sprintf("[image-comfyui] discovered saved workflows profile_id=%s count=%d", resolved.ProfileID, len(catalog.Workflows)))
	return catalog, nil
}

// LoadComfyUIWorkflow imports one fresh successful API snapshot and its
// provider-neutral runtime bindings from the configured ComfyUI server.
func (service *Service) LoadComfyUIWorkflow(ctx context.Context, endpoint config.ImageAPIEndpointSettings, profile config.ImageAPIProfileSettings, workflowPath string) (imagegen.ComfyUIWorkflowSnapshot, error) {
	resolved, err := service.resolveComfyUIConnection(endpoint, profile)
	if err != nil {
		return imagegen.ComfyUIWorkflowSnapshot{}, fmt.Errorf("load ComfyUI workflow: %w", err)
	}
	workflowPath = strings.TrimSpace(workflowPath)
	slog.InfoContext(ctx, fmt.Sprintf("[image-comfyui] loading workflow snapshot profile_id=%s path=%q", resolved.ProfileID, workflowPath))
	snapshot, err := imagegen.NewComfyUIAdapter(nil).LoadWorkflow(ctx, resolved, workflowPath)
	if err != nil {
		return imagegen.ComfyUIWorkflowSnapshot{}, &ProviderRequestError{cause: fmt.Errorf("load ComfyUI workflow: %w", err)}
	}
	slog.InfoContext(ctx, fmt.Sprintf("[image-comfyui] loaded workflow snapshot profile_id=%s workflow_id=%s prompt_candidates=%d", resolved.ProfileID, snapshot.WorkflowID, len(snapshot.Candidates.Prompt)))
	return snapshot, nil
}

func (service *Service) resolveComfyUIConnection(endpoint config.ImageAPIEndpointSettings, profile config.ImageAPIProfileSettings) (config.ResolvedImageAPIProfile, error) {
	if service == nil || service.host == nil {
		return config.ResolvedImageAPIProfile{}, fmt.Errorf("image service host is unavailable")
	}
	host, ok := service.host.(ConfigHost)
	if !ok {
		return config.ResolvedImageAPIProfile{}, fmt.Errorf("configuration snapshot is unavailable")
	}
	snapshot := host.ImageConfigSnapshot()
	resolved, err := config.ResolveImageAPIConnectionEndpointDraft(&snapshot, endpoint, profile)
	if err != nil {
		return config.ResolvedImageAPIProfile{}, err
	}
	if resolved.Protocol != config.ImageProtocolComfyUI {
		return config.ResolvedImageAPIProfile{}, fmt.Errorf("image profile does not use the ComfyUI workflow protocol")
	}
	return resolved, nil
}
