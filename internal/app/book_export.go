package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/book"
	"denova/internal/bookcover"
	"denova/internal/bookexport"
)

// ErrUnsupportedBookExportFormat indicates that the requested export format is not implemented.
var ErrUnsupportedBookExportFormat = errors.New("unsupported book export format")

// BookExportFormat identifies the output format for a book export.
type BookExportFormat string

const (
	// BookExportFormatTXT exports a plain UTF-8 text manuscript.
	BookExportFormatTXT BookExportFormat = "txt"
	// BookExportFormatEPUB exports an EPUB 3.0 file with a chapter table of contents.
	BookExportFormatEPUB BookExportFormat = "epub"
)

// bookExportContentType maps each supported format to its HTTP content type.
var bookExportContentType = map[BookExportFormat]string{
	BookExportFormatTXT:  "text/plain; charset=utf-8",
	BookExportFormatEPUB: "application/epub+zip",
}

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

	// Build the format-independent manuscript once; renderers consume it below.
	manuscript, err := book.NewService(absPath).Manuscript(meta)
	if err != nil {
		return BookExportResult{}, err
	}

	data, err := renderBookExport(absPath, format, manuscript)
	if err != nil {
		return BookExportResult{}, err
	}
	return BookExportResult{
		Filename:     bookExportFilename(meta, absPath, format),
		ContentType:  bookExportContentType[format],
		Data:         data,
		ChapterCount: manuscript.ChapterCount(),
	}, nil
}

// renderBookExport renders the manuscript into the bytes for the requested format.
func renderBookExport(workspace string, format BookExportFormat, manuscript book.Manuscript) ([]byte, error) {
	switch format {
	case BookExportFormatTXT:
		return []byte(bookexport.RenderText(manuscript)), nil
	case BookExportFormatEPUB:
		return bookexport.RenderEPUB(manuscript, readBookCoverPNG(workspace))
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedBookExportFormat, format)
	}
}

// readBookCoverPNG returns the workspace cover image bytes, or nil when the book
// has no cover. Cover embedding is best-effort so a missing cover never blocks
// an EPUB export.
func readBookCoverPNG(workspace string) []byte {
	absCover, err := book.SafePath(workspace, bookcover.CoverPath)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(absCover)
	if err != nil {
		return nil
	}
	return data
}

func normalizeBookExportFormat(format BookExportFormat) BookExportFormat {
	switch BookExportFormat(strings.ToLower(strings.TrimSpace(string(format)))) {
	case BookExportFormatTXT:
		return BookExportFormatTXT
	case BookExportFormatEPUB:
		return BookExportFormatEPUB
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
