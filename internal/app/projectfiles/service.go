package projectfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"denova/internal/book"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

const maxAssetBytes = 32 * 1024 * 1024

var errSymlinkMutation = errors.New("project file operations do not follow symbolic links")

var defaultIgnoredDirectories = map[string]struct{}{
	"build":        {},
	"coverage":     {},
	"dist":         {},
	"node_modules": {},
	"target":       {},
	"vendor":       {},
}

type Service struct {
	registry *projectdomain.Registry
}

type projectRuntime struct {
	record  projectdomain.Record
	layout  projectdomain.Layout
	files   *book.Service
	changes *workspacechange.Service
}

func NewService(registry *projectdomain.Registry) *Service {
	return &Service{registry: registry}
}

func (service *Service) ListDirectory(_ context.Context, projectID, path string, includeIgnored bool) (Directory, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return Directory{}, err
	}
	rel, err := normalizeRelativePath(path, true)
	if err != nil {
		return Directory{}, err
	}
	root, err := os.OpenRoot(runtime.layout.ContentRoot)
	if err != nil {
		return Directory{}, fmt.Errorf("open project directory: %w", err)
	}
	defer root.Close()

	directoryName := "."
	if rel != "" {
		directoryName = filepath.FromSlash(rel)
	}
	directory, err := root.Open(directoryName)
	if err != nil {
		return Directory{}, fmt.Errorf("open project directory %q: %w", rel, err)
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil {
		return Directory{}, fmt.Errorf("inspect project directory %q: %w", rel, err)
	}
	if !info.IsDir() {
		return Directory{}, fmt.Errorf("project path %q is not a directory", rel)
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return Directory{}, fmt.Errorf("list project directory %q: %w", rel, err)
	}

	result := Directory{ProjectID: runtime.record.ID, Path: rel, Entries: make([]Entry, 0, len(entries))}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		ignored := entry.IsDir() && isIgnoredDirectory(name)
		if ignored && !includeIgnored {
			continue
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		entryType := EntryFile
		if entry.IsDir() {
			entryType = EntryDirectory
		}
		result.Entries = append(result.Entries, Entry{
			Name:       name,
			Path:       joinRelativePath(rel, name),
			Type:       entryType,
			Size:       entryInfo.Size(),
			ModifiedAt: entryInfo.ModTime().UTC(),
			Ignored:    ignored,
			Symlink:    entry.Type()&os.ModeSymlink != 0,
		})
	}
	sort.SliceStable(result.Entries, func(left, right int) bool {
		if result.Entries[left].Type != result.Entries[right].Type {
			return result.Entries[left].Type == EntryDirectory
		}
		return strings.ToLower(result.Entries[left].Name) < strings.ToLower(result.Entries[right].Name)
	})
	return result, nil
}

func (service *Service) ReadFile(_ context.Context, projectID, path string) (Document, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return Document{}, err
	}
	rel, err := normalizeRelativePath(path, false)
	if err != nil {
		return Document{}, err
	}
	content, revision, err := runtime.changes.ReadFile(rel)
	if err != nil {
		return Document{}, err
	}
	data := []byte(content)
	mimeType := detectMIMEType(rel, data)
	kind := DocumentText
	editable := true
	if isPreviewableImageMIME(mimeType) {
		kind = DocumentImage
		editable = false
		content = ""
	} else if !utf8.Valid(data) || strings.IndexByte(content, 0) >= 0 {
		kind = DocumentBinary
		editable = false
		content = ""
	}
	return Document{
		ProjectID: runtime.record.ID,
		Path:      rel,
		Content:   content,
		Revision:  revision,
		Kind:      kind,
		MIMEType:  mimeType,
		Size:      len(data),
		Editable:  editable,
	}, nil
}

func (service *Service) ReadAsset(_ context.Context, projectID, path string) ([]byte, string, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return nil, "", err
	}
	rel, err := normalizeRelativePath(path, false)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(runtime.layout.ContentRoot)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("project path %q is not a regular file", rel)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAssetBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxAssetBytes {
		return nil, "", fmt.Errorf("project asset %q exceeds %d bytes", rel, maxAssetBytes)
	}
	contentType := detectMIMEType(rel, data)
	if !isPreviewableImageMIME(contentType) {
		return nil, "", fmt.Errorf("project asset %q is not a previewable image", rel)
	}
	return data, contentType, nil
}

func (service *Service) SaveFile(ctx context.Context, projectID string, request SaveRequest) (SaveResult, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return SaveResult{}, err
	}
	rel, err := normalizeRelativePath(request.Path, false)
	if err != nil {
		return SaveResult{}, err
	}
	saved, err := runtime.changes.SaveFile(ctx, rel, request.Content, request.BaseRevision)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{
		ProjectID: runtime.record.ID,
		Path:      rel,
		Revision:  saved.Revision,
		Changed:   saved.Changed,
	}, nil
}

// ApplyOperations executes every item independently and keeps successful
// results even when neighbours are malformed or fail on disk.
func (service *Service) ApplyOperations(ctx context.Context, projectID string, operations []Operation) ([]OperationResult, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return nil, err
	}
	results := make([]OperationResult, 0, len(operations))
	for _, operation := range operations {
		result := OperationResult{ID: operation.ID, Kind: operation.Kind}
		result.Path, err = service.applyOperation(ctx, runtime, operation)
		if err == nil {
			result.OK = true
		} else {
			result.Code = operationErrorCode(err)
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *Service) applyOperation(ctx context.Context, runtime projectRuntime, operation Operation) (string, error) {
	path, err := normalizeRelativePath(operation.Path, false)
	if err != nil {
		return "", err
	}
	resultPath := path
	err = runtime.changes.WithExclusiveWorkspace(ctx, func() error {
		if pathErr := rejectSymlinkComponents(runtime.layout.ContentRoot, path); pathErr != nil {
			return pathErr
		}
		switch operation.Kind {
		case OperationCreate:
			if operation.Type != "file" && operation.Type != "dir" {
				return fmt.Errorf("create operation type must be file or dir")
			}
			return runtime.files.Create(path, operation.Type, operation.Content)
		case OperationDelete:
			return runtime.files.Delete(path)
		case OperationRename:
			var renameErr error
			resultPath, renameErr = runtime.files.Rename(path, operation.NewName)
			return renameErr
		case OperationCopy:
			to, normalizeErr := normalizeRelativePath(operation.To, false)
			if normalizeErr != nil {
				return normalizeErr
			}
			if symlinkErr := rejectSymlinkComponents(runtime.layout.ContentRoot, to); symlinkErr != nil {
				return symlinkErr
			}
			if symlinkErr := rejectSymlinksBelow(runtime.layout.ContentRoot, path); symlinkErr != nil {
				return symlinkErr
			}
			resultPath = to
			return runtime.files.Copy(path, to)
		case OperationMove:
			to, normalizeErr := normalizeRelativePath(operation.To, false)
			if normalizeErr != nil {
				return normalizeErr
			}
			if symlinkErr := rejectSymlinkComponents(runtime.layout.ContentRoot, to); symlinkErr != nil {
				return symlinkErr
			}
			resultPath = to
			return runtime.files.Move(path, to)
		default:
			return fmt.Errorf("unsupported project file operation %q", operation.Kind)
		}
	})
	return resultPath, err
}

func (service *Service) resolve(projectID string) (projectRuntime, error) {
	if service == nil || service.registry == nil {
		return projectRuntime{}, fmt.Errorf("project registry is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return projectRuntime{}, fmt.Errorf("project ID is required")
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return projectRuntime{}, err
	}
	changes, err := workspacechange.ForWorkspaceAt(layout.ContentRoot, layout.StateRoot)
	if err != nil {
		return projectRuntime{}, err
	}
	return projectRuntime{
		record:  record,
		layout:  layout,
		files:   book.NewService(layout.ContentRoot),
		changes: changes,
	}, nil
}

func normalizeRelativePath(path string, allowRoot bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" && allowRoot {
		return "", nil
	}
	if path == "" {
		return "", fmt.Errorf("project file path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute project file paths are not allowed")
	}
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == "." && allowRoot {
		return "", nil
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project file path is outside the project")
	}
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "" || strings.HasPrefix(component, ".") {
			return "", fmt.Errorf("hidden project paths are not available")
		}
	}
	return filepath.ToSlash(rel), nil
}

func joinRelativePath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func isIgnoredDirectory(name string) bool {
	_, ignored := defaultIgnoredDirectories[strings.ToLower(strings.TrimSpace(name))]
	return ignored
}

func detectMIMEType(path string, content []byte) string {
	if fromExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); fromExtension != "" {
		return fromExtension
	}
	return http.DetectContentType(content)
}

func isPreviewableImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0])) {
	case "image/avif", "image/gif", "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

// rejectSymlinkComponents closes the lexical SafePath gap in the shared Book
// file service. Reads use os.Root directly; unmanaged mutations additionally
// reject existing symlink components before entering that service.
func rejectSymlinkComponents(contentRoot, rel string) error {
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	components := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	for index := range components {
		candidate := filepath.Join(components[:index+1]...)
		info, statErr := root.Lstat(candidate)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errSymlinkMutation, filepath.ToSlash(candidate))
		}
	}
	return nil
}

// CopyPath follows nested symlinks, so copying a directory must reject links
// below the source as well as links in the source path itself.
func rejectSymlinksBelow(contentRoot, rel string) error {
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), rel, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errSymlinkMutation, path)
		}
		return nil
	})
}

func operationErrorCode(err error) string {
	if errors.Is(err, errSymlinkMutation) {
		return "symlink_path"
	}
	if errors.Is(err, os.ErrExist) {
		return "target_exists"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "not_found"
	}
	var changeErr *workspacechange.Error
	if errors.As(err, &changeErr) {
		return changeErr.Code
	}
	return "operation_failed"
}
