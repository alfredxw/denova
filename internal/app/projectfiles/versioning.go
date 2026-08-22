package projectfiles

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"denova/internal/book"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

var ErrBookVersionsUnsupported = errors.New("project does not support Book versions")

// BookVersionResources is the shared versioning projection for one stable
// Book Project. Callers may inspect these dependencies, but visible workspace
// mutations must still pass through the Project change boundary.
type BookVersionResources struct {
	ProjectID      string
	Workspace      string
	StateRoot      string
	Files          *book.Service
	VersionService *book.VersionService
	Settings       book.VersionAutoSettings
}

// BookVersions resolves the one version service shared by file mutations,
// AgentChat runs, and version-management requests for a Project generation.
func (service *Service) BookVersions(projectID string) (BookVersionResources, error) {
	if service == nil {
		return BookVersionResources{}, fmt.Errorf("project-file service is unavailable")
	}
	runtime, err := service.resolve(strings.TrimSpace(projectID))
	if err != nil {
		return BookVersionResources{}, err
	}
	if runtime.record.Type != projectdomain.TypeBook {
		return BookVersionResources{}, ErrBookVersionsUnsupported
	}
	versioning := service.bookMutationVersioning(runtime)
	if versioning.Service == nil {
		return BookVersionResources{}, fmt.Errorf("Book version service is unavailable")
	}
	return BookVersionResources{
		ProjectID: runtime.record.ID, Workspace: runtime.layout.ContentRoot, StateRoot: runtime.layout.StateRoot,
		Files: runtime.files, VersionService: versioning.Service, Settings: versioning.Settings,
	}, nil
}

func (service *Service) VersionStatus(ctx context.Context, projectID string) (book.VersionStatus, error) {
	resources, changes, err := service.bookVersionOperation(projectID)
	if err != nil {
		return book.VersionStatus{}, err
	}
	var status book.VersionStatus
	err = changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var statusErr error
		status, statusErr = resources.VersionService.Status(resources.Settings)
		return statusErr
	})
	return status, err
}

func (service *Service) VersionHistory(projectID string, limit int) ([]book.VersionEntry, error) {
	resources, err := service.BookVersions(projectID)
	if err != nil {
		return nil, err
	}
	return resources.VersionService.History(limit)
}

func (service *Service) CreateVersion(ctx context.Context, projectID, message, source string) (book.VersionCommandResult, error) {
	resources, changes, err := service.bookVersionOperation(projectID)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	var result book.VersionCommandResult
	err = changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var createErr error
		result, createErr = resources.VersionService.Create(message, source, resources.Settings)
		return createErr
	})
	return result, err
}

func (service *Service) VersionDiff(ctx context.Context, projectID, versionID, path string, comparison book.VersionDiffComparison) (book.VersionDiff, error) {
	resources, changes, err := service.bookVersionOperation(projectID)
	if err != nil {
		return book.VersionDiff{}, err
	}
	var diff book.VersionDiff
	err = changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var diffErr error
		diff, diffErr = resources.VersionService.Diff(versionID, path, comparison)
		return diffErr
	})
	return diff, err
}

func (service *Service) VersionRestorePlan(ctx context.Context, projectID, versionID string, paths []string) (book.VersionRestorePlan, error) {
	resources, changes, err := service.bookVersionOperation(projectID)
	if err != nil {
		return book.VersionRestorePlan{}, err
	}
	var plan book.VersionRestorePlan
	err = changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var planErr error
		plan, planErr = resources.VersionService.RestorePlan(versionID, paths, resources.Settings)
		return planErr
	})
	return plan, err
}

func (service *Service) RestoreVersion(ctx context.Context, projectID, versionID string, paths []string) (book.VersionRestoreResult, error) {
	resources, changes, err := service.bookVersionOperation(projectID)
	if err != nil {
		return book.VersionRestoreResult{}, err
	}
	var result book.VersionRestoreResult
	err = changes.WithExclusiveWorkspace(ctx, func() error {
		var restoreErr error
		result, restoreErr = resources.VersionService.RestoreWithPaths(versionID, paths, resources.Settings)
		return restoreErr
	})
	if err == nil {
		resources.VersionService.ScheduleAutoVersion(resources.Settings)
	}
	return result, err
}

func (service *Service) bookVersionOperation(projectID string) (BookVersionResources, *workspacechange.Service, error) {
	resources, err := service.BookVersions(projectID)
	if err != nil {
		return BookVersionResources{}, nil, err
	}
	changes, err := workspacechange.ForWorkspaceAt(resources.Workspace, resources.StateRoot)
	return resources, changes, err
}

// ScheduleBookAutoVersion marks a Project Book dirty using the same scheduler
// and layered settings as project-file writes. General projects intentionally
// have no Book version history and are a no-op.
func (service *Service) ScheduleBookAutoVersion(projectID string) error {
	if service == nil {
		return fmt.Errorf("project-file service is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	runtime, err := service.resolve(projectID)
	if err != nil {
		return err
	}
	if runtime.record.Type != projectdomain.TypeBook {
		return nil
	}
	versioning := service.bookMutationVersioning(runtime)
	if versioning.Service != nil {
		versioning.Service.ScheduleAutoVersion(versioning.Settings)
	}
	return nil
}

// bookVersioningCache owns only schedulers that are not already owned by the
// foreground Writing runtime. Entries follow stable Project identity across
// requests and are replaced when that Project is relinked.
type bookVersioningCache struct {
	mu      sync.Mutex
	closed  bool
	entries map[string]cachedBookVersioning
}

type cachedBookVersioning struct {
	workspace  string
	repository string
	service    *book.VersionService
}

func (service *Service) bookMutationVersioning(runtime projectRuntime) BookMutationVersioning {
	if runtime.record.Type != projectdomain.TypeBook {
		return BookMutationVersioning{}
	}
	settings := book.DefaultVersionAutoSettings()
	if service.bookVersioningProvider != nil {
		hostVersioning := service.bookVersioningProvider.ProjectFileBookMutationVersioning(
			runtime.record.ID,
			runtime.layout.ContentRoot,
			runtime.layout.StateRoot,
		)
		settings = hostVersioning.Settings
		if hostVersioning.Service != nil {
			return hostVersioning
		}
	}
	return BookMutationVersioning{
		Service: service.versions.resolve(
			runtime.record.ID,
			runtime.layout.ContentRoot,
			runtime.layout.VersionRepositoryDir(),
		),
		Settings: settings,
	}
}

func (cache *bookVersioningCache) resolve(projectID, workspace, repository string) *book.VersionService {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		return nil
	}
	if cache.entries == nil {
		cache.entries = make(map[string]cachedBookVersioning)
	}
	cached := cache.entries[projectID]
	if cached.service == nil || filepath.Clean(cached.workspace) != filepath.Clean(workspace) ||
		filepath.Clean(cached.repository) != filepath.Clean(repository) {
		if cached.service != nil {
			cached.service.Close()
		}
		cached = cachedBookVersioning{
			workspace:  workspace,
			repository: repository,
			service:    book.NewVersionService(workspace, repository),
		}
		cache.entries[projectID] = cached
	}
	return cached.service
}

// Close stops idle version timers owned by background Book resources. The
// foreground runtime's service is supplied by the host and is closed there.
func (service *Service) Close() {
	if service == nil {
		return
	}
	service.versions.close()
}

// CloseProject stops background version scheduling tied to one Project
// generation. Relink and archive call this only after the Project lifecycle
// has drained, so no mutation can retain the superseded content directory.
func (service *Service) CloseProject(projectID string) {
	if service == nil {
		return
	}
	service.versions.closeProject(projectID)
}

func (cache *bookVersioningCache) closeProject(projectID string) {
	cache.mu.Lock()
	if cache.entries == nil {
		cache.mu.Unlock()
		return
	}
	cached := cache.entries[projectID]
	delete(cache.entries, projectID)
	cache.mu.Unlock()
	if cached.service != nil {
		cached.service.Close()
	}
}

func (cache *bookVersioningCache) close() {
	cache.mu.Lock()
	if cache.closed {
		cache.mu.Unlock()
		return
	}
	cache.closed = true
	entries := cache.entries
	cache.entries = nil
	cache.mu.Unlock()
	for _, cached := range entries {
		if cached.service != nil {
			cached.service.Close()
		}
	}
}
