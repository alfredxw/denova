package app

import (
	"context"
	"denova/internal/app/resourceexchange"
	"denova/internal/platform"
	workspacechange "denova/internal/workspace/change"
	"fmt"
)

func (a *App) ResourceMarket() *resourceexchange.Market {
	a.ensureServices()
	return a.resourceMarket
}

func (a *App) ResourceExchange() *resourceexchange.Service {
	a.ensureServices()
	return a.resourceExchange
}

func (a *App) ApplyResourcePlan(ctx context.Context, id string) (resourceexchange.Installation, error) {
	service := a.ResourceExchange()
	plan, err := service.ReadPlan(ctx, id)
	if err != nil {
		return resourceexchange.Installation{}, err
	}
	if plan.Installation.ProjectID == "" {
		return a.applyIdleResourcePlan(ctx, service, id)
	}
	var result resourceexchange.Installation
	_, err = a.WithProjectChangeMutation(ctx, plan.Installation.ProjectID, func(changes *workspacechange.Service) (WorkspaceChangeMutationHooks, error) {
		applyErr := changes.WithExclusiveWorkspace(ctx, func() error { var err error; result, err = a.applyIdleResourcePlan(ctx, service, id); return err })
		return WorkspaceChangeMutationHooks{ScheduleAutoVersion: applyErr == nil}, applyErr
	})
	return result, err
}

// InstallExtensionCandidate preserves the existing extension confirmation UI
// while routing all commits through resource ownership and durable recovery.
func (a *App) InstallExtensionCandidate(ctx context.Context, id string, grants []string) (platform.Release, error) {
	plan, err := a.ResourceExchange().PlanExtension(ctx, id, grants)
	if err != nil {
		return platform.Release{}, err
	}
	if _, err := a.ApplyResourcePlan(ctx, plan.ID); err != nil {
		return platform.Release{}, err
	}
	candidate, err := a.platform.CandidateInfo(id)
	if err != nil {
		return platform.Release{}, err
	}
	items, err := a.platform.List(candidate.Kind)
	if err != nil {
		return platform.Release{}, err
	}
	for _, item := range items {
		if item.ID == candidate.Manifest.ID {
			for _, release := range item.Releases {
				if release.Digest == item.CurrentRelease {
					return release, nil
				}
			}
		}
	}
	return platform.Release{}, fmt.Errorf("installed extension release missing")
}

// Keep task registration fenced while a resource set changes. Automatic updates
// defer while consumers run; manual callers receive the same retryable conflict.
func (a *App) applyIdleResourcePlan(ctx context.Context, service *resourceexchange.Service, id string) (resourceexchange.Installation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for task := range a.workspaceTasks {
		select {
		case <-task.Done():
		default:
			return resourceexchange.Installation{}, resourceexchange.ErrResourcesBusy
		}
	}
	for task := range a.projectTasks {
		select {
		case <-task.Done():
		default:
			return resourceexchange.Installation{}, resourceexchange.ErrResourcesBusy
		}
	}
	return service.Apply(ctx, id)
}
