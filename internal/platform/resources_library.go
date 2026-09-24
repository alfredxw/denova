package platform

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
)

// LibraryWrite is an explicit user-authored or user-adopted library mutation.
// An empty baseRevision creates the supplied stable ID. Updating an existing
// item requires the exact last-read revision; image bindings are preserved.
type LibraryWrite struct {
	Item         LibraryItem `json:"item"`
	BaseRevision string      `json:"baseRevision,omitempty"`
	SourceName   string      `json:"sourceName,omitempty"`
	SourceID     string      `json:"sourceId,omitempty"`
	SourceHash   string      `json:"sourceHash,omitempty"`
}

// LibraryWriter is optional so read-only resource hosts remain read-only.
// Implementations must reuse the native library service, preserve its naming
// and revision semantics, and back up any replaced user content.
type LibraryWriter interface {
	WriteLibraryItem(context.Context, string, LibraryWrite) (LibraryItem, error)
}

func (s *ResourceService) serveLibraryWrite(w http.ResponseWriter, request *http.Request, caller *activation, route string) {
	if request.Method != http.MethodPost || route != "/library/items" {
		writeError(w, failure("NOT_FOUND", "Unknown library mutation route"))
		return
	}
	host, ok := s.host.(LibraryWriter)
	if !ok {
		writeError(w, failure("UNSUPPORTED", "Library writing is unavailable"))
		return
	}
	var input LibraryWrite
	if err := readRequest(request, &input); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(input.Item.ID) == "" || len(input.Item.ID) > 200 || strings.ContainsFunc(input.Item.ID, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' }) || strings.TrimSpace(input.Item.Name) == "" || strings.TrimSpace(input.Item.Content) == "" || len(input.Item.Content) > MaxLibraryItemBytes {
		writeError(w, failure("INVALID_ARGUMENT", "Library writing requires a stable ID, name and bounded content"))
		return
	}
	if input.Item.Image != nil {
		writeError(w, failure("INVALID_ARGUMENT", "Library image adoption uses the native image workflow"))
		return
	}
	item, err := host.WriteLibraryItem(request.Context(), caller.context.Scope.ProjectID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	slog.Info("platform_library_item_saved", "package", caller.release.Manifest.ID, "project", caller.context.Scope.ProjectID, "item", item.ID)
	writeResponse(w, http.StatusOK, item)
}
