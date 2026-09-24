package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"denova/internal/portablepath"
	"denova/internal/revisionfile"
)

// ContentDocument is an extension-owned authoring artifact. It is shared only
// within one Project and package, never a replacement for an Agent journal.
// Revisions use the existing file compare-and-swap contract.
type ContentDocument struct {
	Path             string  `json:"path"`
	Content          string  `json:"content,omitempty"`
	ExpectedRevision *string `json:"expectedRevision,omitempty" jsonschema:"nullable" jsonschema_description:"Absent or null creates only; a nonempty revision performs compare-and-swap."`
}

func sharedContentDirectory(runtime *Runtime, caller *activation) (string, error) {
	_, layout, err := runtime.manager.registry.Resolve(caller.context.Scope.ProjectID, true)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.StoreRoot, "extensions", caller.release.Manifest.ID, "content"), nil
}

func (s *ResourceService) serveSharedContent(w http.ResponseWriter, request *http.Request, runtime *Runtime, caller *activation, route string) {
	if route == "/assets/source" {
		s.serveContentSource(w, request, runtime)
		return
	}
	directory, err := sharedContentDirectory(runtime, caller)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := portablepath.PreflightTree(directory); err != nil && !os.IsNotExist(err) {
		writeError(w, err)
		return
	}
	switch {
	case route == "/assets/upload" && request.Method == http.MethodPost:
		data, err := io.ReadAll(io.LimitReader(request.Body, MaxAssetBytes+1))
		if err != nil {
			writeError(w, err)
			return
		}
		mimeType, extension, err := mediaType(data)
		if err != nil {
			writeError(w, err)
			return
		}
		sum := sha256.Sum256(data)
		name := hex.EncodeToString(sum[:]) + extension
		file := filepath.Join(directory, "assets", name)
		_, err = revisionfile.ReplaceIfRevision(request.Context(), file, revisionfile.MissingRevision, data, revisionfile.Options{FileMode: 0600, DirectoryMode: 0700})
		if err != nil && !errors.Is(err, revisionfile.ErrRevisionConflict) {
			writeError(w, err)
			return
		}
		if err != nil {
			previous, readErr := os.ReadFile(file)
			if readErr != nil || !bytes.Equal(previous, data) {
				writeError(w, failure("DOCUMENT_CONFLICT", "Stored media no longer matches its content address"))
				return
			}
		}
		slog.Info("platform_media_adopted", "package", caller.release.Manifest.ID, "project", caller.context.Scope.ProjectID, "asset", name)
		writeResponse(w, http.StatusOK, GeneratedImage{Asset: AssetRef{Kind: "shared", Path: name}, MIMEType: mimeType, SizeBytes: len(data)})
	case route == "/assets/content" && request.Method == http.MethodGet:
		name := request.URL.Query().Get("path")
		assetDir := filepath.Join(directory, "assets")
		if err := portablepath.CheckNoCollision(assetDir, name); err != nil || strings.Contains(name, "/") {
			writeError(w, failure("INVALID_ARGUMENT", "Shared media requires a content-addressed filename"))
			return
		}
		data, err := os.ReadFile(filepath.Join(assetDir, name))
		if err != nil {
			writeError(w, err)
			return
		}
		mimeType, _, err := mediaType(data)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	case route == "/assets/documents" && request.Method == http.MethodGet:
		entries, err := os.ReadDir(filepath.Join(directory, "documents"))
		if err != nil && !os.IsNotExist(err) {
			writeError(w, err)
			return
		}
		names := []string{}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
				names = append(names, entry.Name())
			}
		}
		slices.Sort(names)
		writeResponse(w, http.StatusOK, map[string]any{"items": names})
	case route == "/assets/document":
		s.serveContentDocument(w, request, filepath.Join(directory, "documents"))
	default:
		writeError(w, failure("NOT_FOUND", "Unknown shared content route"))
	}
}

func (s *ResourceService) serveContentDocument(w http.ResponseWriter, request *http.Request, directory string) {
	var input ContentDocument
	if request.Method == http.MethodPut {
		if err := readRequest(request, &input); err != nil {
			writeError(w, err)
			return
		}
	} else if request.Method == http.MethodGet {
		input.Path = request.URL.Query().Get("path")
	} else {
		writeError(w, failure("NOT_FOUND", "Unknown content document method"))
		return
	}
	if err := portablepath.CheckNoCollision(directory, input.Path); err != nil || strings.Contains(input.Path, "/") || !strings.HasSuffix(input.Path, ".json") {
		writeError(w, failure("INVALID_ARGUMENT", "Content documents require a portable JSON filename"))
		return
	}
	file := filepath.Join(directory, input.Path)
	if request.Method == http.MethodGet {
		result, err := revisionfile.Read(request.Context(), file)
		if err != nil {
			writeError(w, err)
			return
		}
		if !result.Exists {
			writeError(w, failure("NOT_FOUND", "Content document does not exist"))
			return
		}
		writeResponse(w, http.StatusOK, FileSnapshot{Content: string(result.Content), Revision: result.Revision})
		return
	}
	if !json.Valid([]byte(input.Content)) || len(input.Content) > MaxDefinitionBytes/2 {
		writeError(w, failure("INVALID_ARGUMENT", "Content document requires bounded JSON content"))
		return
	}
	expected := revisionfile.MissingRevision
	if input.ExpectedRevision != nil {
		expected = *input.ExpectedRevision
	}
	result, err := revisionfile.ReplaceIfRevision(request.Context(), file, expected, []byte(input.Content), revisionfile.Options{FileMode: 0600, DirectoryMode: 0700})
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, http.StatusOK, map[string]string{"revision": result.Revision})
}

func mediaType(data []byte) (string, string, error) {
	if len(data) > MaxAssetBytes {
		return "", "", failure("LIMIT_EXCEEDED", "Media exceeds %d bytes", MaxAssetBytes)
	}
	mimeType := http.DetectContentType(data)
	extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif", "audio/wave": ".wav", "audio/x-wav": ".wav", "audio/mpeg": ".mp3", "audio/ogg": ".ogg", "application/ogg": ".ogg", "audio/flac": ".flac"}
	extension := extensions[mimeType]
	if extension == "" {
		return "", "", failure("UNSUPPORTED", "Media must be a supported raster image or audio file")
	}
	return mimeType, extension, nil
}
