package projectfiles

import (
	"path/filepath"
	"sync"

	"denova/internal/book"
	projectdomain "denova/internal/project"
)

// bookVersioningCache owns only schedulers that are not already owned by the
// foreground Writing runtime. Entries follow stable Project identity across
// requests and are replaced when that Project is relinked.
type bookVersioningCache struct {
	mu      sync.Mutex
	closed  bool
	entries map[string]cachedBookVersioning
}

type cachedBookVersioning struct {
	workspace string
	service   *book.VersionService
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
		Service:  service.versions.resolve(runtime.record.ID, runtime.layout.ContentRoot),
		Settings: settings,
	}
}

func (cache *bookVersioningCache) resolve(projectID, workspace string) *book.VersionService {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		return nil
	}
	if cache.entries == nil {
		cache.entries = make(map[string]cachedBookVersioning)
	}
	cached := cache.entries[projectID]
	if cached.service == nil || filepath.Clean(cached.workspace) != filepath.Clean(workspace) {
		if cached.service != nil {
			cached.service.Close()
		}
		cached = cachedBookVersioning{
			workspace: workspace,
			service:   book.NewVersionService(workspace),
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
