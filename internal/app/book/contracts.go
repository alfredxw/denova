// Package book owns application-level book assets and export operations. Book
// content remains in the book domain, while the project registry supplies the
// stable content-to-state layout needed for layered settings.
package bookapp

import (
	"fmt"
	"path/filepath"

	bookdomain "denova/internal/book"
	projectdomain "denova/internal/project"
)

type Service struct {
	dataDir  string
	registry *projectdomain.Registry
	metadata *bookdomain.MetaStore
}

func NewService(dataDir string, registry *projectdomain.Registry, metadata *bookdomain.MetaStore) *Service {
	return &Service{dataDir: dataDir, registry: registry, metadata: metadata}
}

func (service *Service) metadataLayout(path string) (projectdomain.Layout, error) {
	if service == nil || service.registry == nil {
		return projectdomain.Layout{}, fmt.Errorf("project registry is unavailable")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return projectdomain.Layout{}, fmt.Errorf("invalid Book path: %w", err)
	}
	_, layout, err := service.registry.ResolveByPath(absolute, true)
	return layout, err
}
