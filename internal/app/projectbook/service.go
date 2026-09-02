// Package projectbook owns Book-specific resources addressed by stable Project ID.
// Operations resolve the current content directory for every request and never
// activate or otherwise mutate the foreground Writing workspace.
package projectbook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	reviewapp "denova/internal/app/review"
	"denova/internal/book"
	booklore "denova/internal/book/lore"
	projectdomain "denova/internal/project"
	"denova/internal/workspace/documentreview"
)

var ErrBookProjectRequired = errors.New("project is not a Book")

// Snapshot is the shared read model used by Writing and embedded project pages.
type Snapshot struct {
	ProjectID string                `json:"project_id"`
	Workspace string                `json:"workspace"`
	Tree      []*book.FileNode      `json:"tree"`
	Summary   book.WorkspaceSummary `json:"summary"`
}

// Service keeps all Book-only resource resolution behind one Project-ID seam.
// File content and assets remain in projectfiles because those also support
// General projects.
type Service struct {
	registry *projectdomain.Registry
	mu       sync.Mutex
	files    map[string]cachedBookFiles
	sequence uint64
}

func NewService(registry *projectdomain.Registry) *Service {
	return &Service{registry: registry, files: make(map[string]cachedBookFiles)}
}

func (service *Service) Snapshot(ctx context.Context, projectID string) (Snapshot, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	tree, err := runtime.files.Tree()
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	summary, err := runtime.files.Summary()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		ProjectID: runtime.record.ID,
		Workspace: runtime.layout.ContentRoot,
		Tree:      tree,
		Summary:   summary,
	}, nil
}

func (service *Service) Tree(ctx context.Context, projectID string) (string, []*book.FileNode, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return "", nil, err
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	tree, err := runtime.files.Tree()
	return runtime.layout.ContentRoot, tree, err
}

func (service *Service) Summary(ctx context.Context, projectID string) (string, book.WorkspaceSummary, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return "", book.WorkspaceSummary{}, err
	}
	if err := ctx.Err(); err != nil {
		return "", book.WorkspaceSummary{}, err
	}
	summary, err := runtime.files.Summary()
	return runtime.layout.ContentRoot, summary, err
}

func (service *Service) SetChapterConfirmed(projectID, path string, confirmed bool) error {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return err
	}
	return runtime.files.SetChapterConfirmed(path, confirmed)
}

func (service *Service) LoreItems(projectID string) ([]booklore.Item, error) {
	store, err := service.loreStore(projectID)
	if err != nil {
		return nil, err
	}
	return store.ListAll()
}

func (service *Service) CreateLoreItem(projectID string, input booklore.ItemInput) (booklore.Item, error) {
	store, err := service.loreStore(projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	return store.Create(input)
}

func (service *Service) UpdateLoreItem(projectID, id string, input booklore.ItemInput) (booklore.Item, error) {
	store, err := service.loreStore(projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	return store.Update(id, input)
}

func (service *Service) DeleteLoreItem(projectID, id string) error {
	store, err := service.loreStore(projectID)
	if err != nil {
		return err
	}
	return store.Delete(id)
}

// WithDocumentReview resolves the review ledger and target snapshots from the
// same Book runtime, so comments can never bind to a different foreground Book.
func (service *Service) WithDocumentReview(
	projectID string,
	action func(*documentreview.Service, documentreview.SnapshotResolver) error,
) (string, error) {
	if action == nil {
		return "", errors.New("document review action is nil")
	}
	runtime, err := service.resolve(projectID)
	if err != nil {
		return "", err
	}
	reviews, err := documentreview.ForWorkspaceAt(runtime.layout.ContentRoot, runtime.layout.StoreRoot)
	if err != nil {
		return "", err
	}
	resolver := reviewapp.NewTargetResolver(runtime.layout.ContentRoot, runtime.files)
	if err := action(reviews, resolver); err != nil {
		return "", err
	}
	return runtime.layout.ContentRoot, nil
}

type runtime struct {
	record projectdomain.Record
	layout projectdomain.Layout
	files  *book.Service
}

func (service *Service) loreStore(projectID string) (*booklore.Store, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return nil, err
	}
	return booklore.NewStore(runtime.layout.ContentRoot), nil
}

func (service *Service) resolve(projectID string) (runtime, error) {
	if service == nil || service.registry == nil {
		return runtime{}, fmt.Errorf("project registry is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return runtime{}, fmt.Errorf("project ID is required")
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return runtime{}, err
	}
	if record.Type != projectdomain.TypeBook {
		return runtime{}, fmt.Errorf("%w: project_id=%s", ErrBookProjectRequired, record.ID)
	}
	return runtime{record: record, layout: layout, files: service.bookFiles(record.ID, layout.ContentRoot)}, nil
}

const bookFilesCacheLimit = 8

type cachedBookFiles struct {
	workspace string
	files     *book.Service
	used      uint64
}

func (service *Service) bookFiles(projectID, workspace string) *book.Service {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sequence++
	if cached, exists := service.files[projectID]; exists && cached.workspace == workspace {
		cached.used = service.sequence
		service.files[projectID] = cached
		return cached.files
	}
	files := book.NewService(workspace)
	service.files[projectID] = cachedBookFiles{workspace: workspace, files: files, used: service.sequence}
	if len(service.files) <= bookFilesCacheLimit {
		return files
	}
	oldestID := ""
	oldestUse := service.sequence
	for id, candidate := range service.files {
		if id != projectID && candidate.used <= oldestUse {
			oldestID, oldestUse = id, candidate.used
		}
	}
	delete(service.files, oldestID)
	return files
}

// InvalidateSummary forwards an ephemeral filesystem invalidation to an
// already-open Book projection. Cold projects need no work: their first read
// builds from canonical files.
func (service *Service) InvalidateSummary(projectID string, paths []string, resync bool) {
	if service == nil {
		return
	}
	service.mu.Lock()
	cached, exists := service.files[strings.TrimSpace(projectID)]
	service.mu.Unlock()
	if exists {
		cached.files.InvalidateSummary(paths, resync)
	}
}
