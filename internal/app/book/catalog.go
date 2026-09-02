package bookapp

import (
	"fmt"
	"log/slog"
	"time"

	bookdomain "denova/internal/book"
	projectdomain "denova/internal/project"
)

// Record is the stable projection shared by every Book selection surface.
type Record struct {
	ProjectID      string `json:"project_id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Author         string `json:"author"`
	CoverUpdatedAt string `json:"cover_updated_at,omitempty"`
	LastOpenedAt   string `json:"last_opened_at"`
}

// Books lists available Book projects enriched with user metadata and cover
// freshness. Project identity and ordering remain owned by project.Registry.
func (service *Service) Books() []Record {
	if service == nil || service.registry == nil {
		return []Record{}
	}
	projects, err := service.registry.Books()
	if err != nil {
		slog.Error(fmt.Sprintf("[app/book] list Books failed err=%v", err))
		return []Record{}
	}
	records := make([]Record, 0, len(projects))
	for _, project := range projects {
		record := bookRecord(project)
		if service.metadata != nil {
			layout, layoutErr := service.registry.Layout(project)
			if layoutErr == nil {
				if metadata, readErr := service.metadata.Read(layout.ContentRoot, layout.StoreRoot); readErr == nil {
					if metadata.Title != "" {
						record.Name = metadata.Title
					}
					record.Author = metadata.Author
				}
			}
		}
		record.CoverUpdatedAt = CoverUpdatedAt(record.Path)
		records = append(records, record)
	}
	return records
}

func (service *Service) SortMode() projectdomain.SortMode {
	if service == nil || service.registry == nil {
		return ""
	}
	return service.registry.SortMode()
}

func (service *Service) Info(path string) (bookdomain.BookMeta, error) {
	if service == nil || service.metadata == nil {
		return bookdomain.BookMeta{}, fmt.Errorf("Book metadata store is unavailable")
	}
	layout, err := service.metadataLayout(path)
	if err != nil {
		return bookdomain.BookMeta{}, err
	}
	return service.metadata.Read(layout.ContentRoot, layout.StoreRoot)
}

func (service *Service) UpdateInfo(path, title, author, description string) (bookdomain.BookMeta, error) {
	metadata, err := service.Info(path)
	if err != nil {
		return bookdomain.BookMeta{}, err
	}
	if title != "" {
		metadata.Title = title
	}
	metadata.Author = author
	metadata.Description = description
	layout, err := service.metadataLayout(path)
	if err != nil {
		return bookdomain.BookMeta{}, err
	}
	return service.metadata.Write(layout.ContentRoot, layout.StoreRoot, metadata)
}

func (service *Service) Reorder(paths []string) error {
	if service == nil || service.registry == nil {
		return fmt.Errorf("project registry is unavailable")
	}
	return service.registry.ReorderBooks(paths)
}

func (service *Service) SetSortMode(mode projectdomain.SortMode) error {
	if service == nil || service.registry == nil {
		return fmt.Errorf("project registry is unavailable")
	}
	return service.registry.SetSortMode(mode)
}

func bookRecord(project projectdomain.Record) Record {
	openedAt := ""
	if !project.LastOpenedAt.IsZero() {
		openedAt = project.LastOpenedAt.Format(time.RFC3339Nano)
	}
	return Record{ProjectID: project.ID, Name: project.Name, Path: project.WorkspacePath, LastOpenedAt: openedAt}
}
