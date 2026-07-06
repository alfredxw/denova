package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/book"
)

// ErrUnsupportedBookExportFormat indicates that the requested export format is not implemented.
var ErrUnsupportedBookExportFormat = errors.New("unsupported book export format")

// BookExportFormat identifies the output format for a book export.
type BookExportFormat string

const (
	// BookExportFormatTXT exports a plain UTF-8 text manuscript.
	BookExportFormatTXT BookExportFormat = "txt"
)

// BookExportRequest describes a format-specific book export request.
type BookExportRequest struct {
	Path   string           `json:"path"`
	Format BookExportFormat `json:"format"`
}

// BookExportResult carries a generated export file back to the API layer.
type BookExportResult struct {
	Filename     string
	ContentType  string
	Data         []byte
	ChapterCount int
}

// ExportBook exports a book workspace in the requested format.
func (a *App) ExportBook(req BookExportRequest) (BookExportResult, error) {
	return a.runtime().ExportBook(req)
}

func (s *WorkspaceRuntimeManager) ExportBook(req BookExportRequest) (BookExportResult, error) {
	format := normalizeBookExportFormat(req.Format)
	if format == "" {
		return BookExportResult{}, fmt.Errorf("%w: %s", ErrUnsupportedBookExportFormat, req.Format)
	}
	absPath, err := validateBookWorkspacePath(req.Path)
	if err != nil {
		return BookExportResult{}, err
	}
	meta, err := s.app.bookMetaStore.Read(absPath)
	if err != nil {
		return BookExportResult{}, err
	}

	switch format {
	case BookExportFormatTXT:
		result, err := book.NewService(absPath).ExportText(meta)
		if err != nil {
			return BookExportResult{}, err
		}
		return BookExportResult{
			Filename:     bookExportFilename(meta, absPath, format),
			ContentType:  "text/plain; charset=utf-8",
			Data:         []byte(result.Content),
			ChapterCount: result.ChapterCount,
		}, nil
	default:
		return BookExportResult{}, fmt.Errorf("%w: %s", ErrUnsupportedBookExportFormat, req.Format)
	}
}

func normalizeBookExportFormat(format BookExportFormat) BookExportFormat {
	switch BookExportFormat(strings.ToLower(strings.TrimSpace(string(format)))) {
	case BookExportFormatTXT:
		return BookExportFormatTXT
	default:
		return ""
	}
}

func bookExportFilename(meta book.BookMeta, workspace string, format BookExportFormat) string {
	name := strings.TrimSpace(meta.Title)
	if name == "" {
		name = filepath.Base(workspace)
	}
	name = sanitizeDownloadFilenameBase(name)
	if name == "" {
		name = "book"
	}
	return name + "." + string(format)
}

func sanitizeDownloadFilenameBase(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, value)
	return strings.Trim(value, ". ")
}
