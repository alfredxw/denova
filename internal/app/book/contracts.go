// Package book owns application-level book assets and export operations. Book
// content remains in the book domain, while the project registry supplies the
// stable content-to-state layout needed for layered settings.
package bookapp

import (
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
