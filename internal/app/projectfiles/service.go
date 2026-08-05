package projectfiles

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
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

const (
	maxAssetBytes = 32 * 1024 * 1024
	// DefaultTreeEntryLimit intentionally sits far above an ordinary creator
	// project. It bounds response size without turning normal directory depth
	// into repeated network round trips.
	DefaultTreeEntryLimit    = 100_000
	MaximumTreeEntryLimit    = 1_000_000
	maximumTreeResolveTarget = 256
	maximumResolvedDirChain  = 64
)

var errSymlinkMutation = errors.New("project file operations do not follow symbolic links")

type symlinkPathBoundary uint8

const (
	symlinkPathIncludesLeaf symlinkPathBoundary = iota
	symlinkPathParentsOnly
)

var defaultIgnoredDirectories = map[string]struct{}{
	"build":        {},
	"coverage":     {},
	"dist":         {},
	"node_modules": {},
	"target":       {},
	"vendor":       {},
}

type Service struct {
	registry               *projectdomain.Registry
	bookVersioningProvider BookMutationVersioningProvider
	treeEntryLimit         int
	versions               bookVersioningCache
}

// ServiceOption configures a project-file service without exposing its
// internal cache and filesystem boundaries.
type ServiceOption func(*Service)

// WithTreeEntryLimit sets the per-request ceiling used by recursive tree
// resolution. Invalid values fall back to the safe default and values above
// the hard ceiling are clamped.
func WithTreeEntryLimit(limit int) ServiceOption {
	return func(service *Service) {
		service.treeEntryLimit = normalizeTreeEntryLimit(limit)
	}
}

// BookMutationVersioning preserves Writing's version-history contract while
// the project-scoped file service remains usable for non-Book projects.
type BookMutationVersioning struct {
	Service  *book.VersionService
	Settings book.VersionAutoSettings
}

// BookMutationVersioningProvider resolves version behavior without coupling
// this project-scoped service to the foreground App package.
type BookMutationVersioningProvider interface {
	ProjectFileBookMutationVersioning(projectID, workspace, stateRoot string) BookMutationVersioning
}

type projectRuntime struct {
	record  projectdomain.Record
	layout  projectdomain.Layout
	files   *book.Service
	changes *workspacechange.Service
}

func NewService(registry *projectdomain.Registry, options ...ServiceOption) *Service {
	return newService(registry, nil, options...)
}

// NewServiceWithBookVersioning reuses a foreground Book scheduler when the host
// has one and owns a scoped scheduler for every background Book it mutates.
func NewServiceWithBookVersioning(registry *projectdomain.Registry, provider BookMutationVersioningProvider, options ...ServiceOption) *Service {
	return newService(registry, provider, options...)
}

func newService(registry *projectdomain.Registry, provider BookMutationVersioningProvider, options ...ServiceOption) *Service {
	service := &Service{
		registry:               registry,
		bookVersioningProvider: provider,
		treeEntryLimit:         DefaultTreeEntryLimit,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// ResolveTree resolves several explorer branches in one bounded request. Each
// target is isolated so a stale restored path does not discard valid siblings.
func (service *Service) ResolveTree(ctx context.Context, projectID string, request TreeResolveRequest) (TreeResolveResponse, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return TreeResolveResponse{}, err
	}
	if len(request.Targets) == 0 {
		return TreeResolveResponse{}, fmt.Errorf("at least one project tree target is required")
	}
	if len(request.Targets) > maximumTreeResolveTarget {
		return TreeResolveResponse{}, fmt.Errorf("project tree request exceeds %d targets", maximumTreeResolveTarget)
	}

	budget := resolveTreeEntryBudget(request.EntryBudget, service.treeEntryLimit)
	root, err := os.OpenRoot(runtime.layout.ContentRoot)
	if err != nil {
		return TreeResolveResponse{}, fmt.Errorf("open project directory: %w", err)
	}
	defer root.Close()

	response := TreeResolveResponse{
		ProjectID: runtime.record.ID,
		Results:   make([]TreeResolveResult, 0, len(request.Targets)),
	}
	remainingBudget := budget
	for index, target := range request.Targets {
		if err := ctx.Err(); err != nil {
			return TreeResolveResponse{}, err
		}
		if remainingBudget == 0 {
			response.Results = append(response.Results, TreeResolveResult{
				ID:    target.ID,
				Path:  strings.TrimSpace(target.Path),
				Code:  "budget_exhausted",
				Error: "project tree entry budget was exhausted",
			})
			continue
		}
		remainingTargets := len(request.Targets) - index
		targetBudget := remainingBudget / remainingTargets
		if targetBudget == 0 {
			targetBudget = 1
		}
		result, used := resolveTreeTarget(
			ctx,
			root,
			target,
			request.IncludeIgnored,
			request.FollowSingleChildDirectories,
			request.Recursive,
			targetBudget,
		)
		if err := ctx.Err(); err != nil {
			return TreeResolveResponse{}, err
		}
		remainingBudget -= used
		if remainingBudget < 0 {
			remainingBudget = 0
		}
		response.Results = append(response.Results, result)
		if request.Recursive {
			attributes := []any{
				"project_id", runtime.record.ID,
				"target_path", result.Path,
				"resolved_entries", used,
				"resolved_directories", len(result.Directories),
				"entry_limit", targetBudget,
			}
			if recursiveTreeResultComplete(result) {
				slog.DebugContext(ctx, "[internal/app/projectfiles/service.go] recursively resolved project file tree", attributes...)
			} else if result.OK {
				slog.InfoContext(ctx, "[internal/app/projectfiles/service.go] recursive project file tree reached its boundary; leaving branches for on-demand loading", attributes...)
			}
		}
	}
	return response, nil
}

type treeCursor struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Offset   int    `json:"offset"`
}

type treeResolveFailure struct {
	code string
	err  error
}

func (failure *treeResolveFailure) Error() string { return failure.err.Error() }
func (failure *treeResolveFailure) Unwrap() error { return failure.err }

func resolveTreeTarget(
	ctx context.Context,
	root *os.Root,
	target TreeResolveTarget,
	includeIgnored bool,
	followSingleChildDirectories bool,
	recursive bool,
	budget int,
) (TreeResolveResult, int) {
	result := TreeResolveResult{ID: target.ID, Path: strings.TrimSpace(target.Path)}
	path, err := normalizeRelativePath(target.Path, true)
	if err != nil {
		result.Code = "invalid_path"
		result.Error = err.Error()
		return result, 0
	}
	result.Path = path
	if recursive {
		return resolveTreeTargetRecursive(ctx, root, result, path, strings.TrimSpace(target.Cursor), includeIgnored, budget)
	}
	currentPath := path
	cursor := strings.TrimSpace(target.Cursor)
	used := 0
	resolvedDirectories := 0
	for budget > 0 && resolvedDirectories < maximumResolvedDirChain {
		page, pageErr := readDirectoryPage(ctx, root, currentPath, includeIgnored, cursor, budget)
		if pageErr != nil {
			result.Code = treeResolveErrorCode(pageErr)
			result.Error = pageErr.Error()
			return result, used
		}
		result.Directories = append(result.Directories, page)
		resolvedDirectories++
		used += len(page.Entries)
		budget -= len(page.Entries)
		if cursor != "" || !followSingleChildDirectories || page.ChildrenState == DirectoryChildrenPartial || len(page.Entries) != 1 {
			break
		}
		child := page.Entries[0]
		if child.Type != EntryDirectory || child.Symlink {
			break
		}
		currentPath = child.Path
	}
	result.OK = true
	return result, used
}

func readDirectoryPage(ctx context.Context, root *os.Root, path string, includeIgnored bool, encodedCursor string, budget int) (DirectoryPage, error) {
	directoryName := "."
	if path != "" {
		directoryName = filepath.FromSlash(path)
	}
	directory, err := root.Open(directoryName)
	if err != nil {
		return DirectoryPage{}, classifyTreeReadError(path, err)
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil {
		return DirectoryPage{}, classifyTreeReadError(path, err)
	}
	if !info.IsDir() {
		return DirectoryPage{}, &treeResolveFailure{code: "not_directory", err: fmt.Errorf("project path %q is not a directory", path)}
	}
	directoryEntries, err := directory.ReadDir(-1)
	if err != nil {
		return DirectoryPage{}, classifyTreeReadError(path, err)
	}

	entries := make([]Entry, 0, len(directoryEntries))
	for index, directoryEntry := range directoryEntries {
		if index%256 == 0 {
			if contextErr := ctx.Err(); contextErr != nil {
				return DirectoryPage{}, contextErr
			}
		}
		name := directoryEntry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		ignored := directoryEntry.IsDir() && isIgnoredDirectory(name)
		if ignored && !includeIgnored {
			continue
		}
		entryType := EntryFile
		if directoryEntry.IsDir() {
			entryType = EntryDirectory
		}
		entries = append(entries, Entry{
			Name:    name,
			Path:    joinRelativePath(path, name),
			Type:    entryType,
			Ignored: ignored,
			Symlink: directoryEntry.Type()&os.ModeSymlink != 0,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Type != entries[right].Type {
			return entries[left].Type == EntryDirectory
		}
		return book.CompareFileNodeNames(entries[left].Name, entries[right].Name) < 0
	})
	if err := ctx.Err(); err != nil {
		return DirectoryPage{}, err
	}
	revision := directoryRevision(entries)
	offset := 0
	if encodedCursor != "" {
		cursor, cursorErr := decodeTreeCursor(encodedCursor)
		if cursorErr != nil || cursor.Path != path || cursor.Offset < 0 || cursor.Offset > len(entries) {
			return DirectoryPage{}, &treeResolveFailure{code: "invalid_cursor", err: fmt.Errorf("invalid project tree continuation for %q", path)}
		}
		if cursor.Revision != revision {
			return DirectoryPage{}, &treeResolveFailure{code: "cursor_stale", err: fmt.Errorf("project directory %q changed while loading", path)}
		}
		offset = cursor.Offset
	}
	end := offset + budget
	if end > len(entries) {
		end = len(entries)
	}
	page := DirectoryPage{
		Path:          path,
		Revision:      revision,
		Entries:       append([]Entry(nil), entries[offset:end]...),
		ChildrenState: DirectoryChildrenComplete,
	}
	if end < len(entries) {
		page.ChildrenState = DirectoryChildrenPartial
		page.Continuation = encodeTreeCursor(treeCursor{Path: path, Revision: revision, Offset: end})
	}
	return page, nil
}

func directoryRevision(entries []Entry) string {
	hash := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(hash, "%s\x00%s\x00%t\x00%t\n", entry.Name, entry.Type, entry.Ignored, entry.Symlink)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

func encodeTreeCursor(cursor treeCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeTreeCursor(encoded string) (treeCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return treeCursor{}, err
	}
	var cursor treeCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return treeCursor{}, err
	}
	return cursor, nil
}

func classifyTreeReadError(path string, err error) error {
	code := "read_failed"
	if errors.Is(err, os.ErrNotExist) {
		code = "not_found"
	}
	return &treeResolveFailure{code: code, err: fmt.Errorf("read project directory %q: %w", path, err)}
}

func treeResolveErrorCode(err error) string {
	var failure *treeResolveFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return "read_failed"
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
	if saved.Changed {
		versioning := service.bookMutationVersioning(runtime)
		if versioning.Service != nil {
			versioning.Service.ScheduleAutoVersion(versioning.Settings)
		}
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
	versioning := service.bookMutationVersioning(runtime)
	changed := false
	results := make([]OperationResult, 0, len(operations))
	for _, operation := range operations {
		result := OperationResult{ID: operation.ID, Kind: operation.Kind}
		result.Path, err = service.applyOperation(ctx, runtime, versioning, operation)
		if err == nil {
			result.OK = true
			changed = true
		} else {
			result.Code = operationErrorCode(err)
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	if changed && versioning.Service != nil {
		versioning.Service.ScheduleAutoVersion(versioning.Settings)
	}
	return results, nil
}

func (service *Service) applyOperation(
	ctx context.Context,
	runtime projectRuntime,
	versioning BookMutationVersioning,
	operation Operation,
) (string, error) {
	path, err := normalizeRelativePath(operation.Path, false)
	if err != nil {
		return "", err
	}
	resultPath := path
	err = runtime.changes.WithExclusiveWorkspace(ctx, func() error {
		switch operation.Kind {
		case OperationCreate:
			if pathErr := rejectSymlinkComponents(runtime.layout.ContentRoot, path); pathErr != nil {
				return pathErr
			}
			if operation.Type != "file" && operation.Type != "dir" {
				return fmt.Errorf("create operation type must be file or dir")
			}
			return runtime.files.Create(path, operation.Type, operation.Content)
		case OperationDelete:
			if pathErr := rejectSymlinkParents(runtime.layout.ContentRoot, path); pathErr != nil {
				return pathErr
			}
			if versioning.Service != nil {
				if _, versionErr := versioning.Service.Create("删除前自动备份", book.VersionSourceManual, versioning.Settings); versionErr != nil && !errors.Is(versionErr, book.ErrVersionClean) {
					return versionErr
				}
			}
			return runtime.files.Delete(path)
		case OperationRename:
			if pathErr := rejectSymlinkParents(runtime.layout.ContentRoot, path); pathErr != nil {
				return pathErr
			}
			if validateErr := book.ValidateNewName(operation.NewName); validateErr != nil {
				return validateErr
			}
			renameTarget := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(path)), operation.NewName))
			if symlinkErr := rejectSymlinkComponents(runtime.layout.ContentRoot, renameTarget); symlinkErr != nil {
				return symlinkErr
			}
			var renameErr error
			resultPath, renameErr = runtime.files.Rename(path, operation.NewName)
			return renameErr
		case OperationCopy:
			if pathErr := rejectSymlinkComponents(runtime.layout.ContentRoot, path); pathErr != nil {
				return pathErr
			}
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
			if pathErr := rejectSymlinkParents(runtime.layout.ContentRoot, path); pathErr != nil {
				return pathErr
			}
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
	raw := filepath.FromSlash(path)
	for _, component := range strings.Split(raw, string(filepath.Separator)) {
		if component == ".." {
			return "", fmt.Errorf("project file path cannot contain parent directory segments")
		}
	}
	rel := filepath.Clean(raw)
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
	return rejectSymlinkPath(contentRoot, rel, symlinkPathIncludesLeaf)
}

// rejectSymlinkParents allows safe operations on a link itself while still
// preventing an ancestor link from redirecting the mutation outside the tree.
func rejectSymlinkParents(contentRoot, rel string) error {
	return rejectSymlinkPath(contentRoot, rel, symlinkPathParentsOnly)
}

func rejectSymlinkPath(contentRoot, rel string, boundary symlinkPathBoundary) error {
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	components := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	switch boundary {
	case symlinkPathIncludesLeaf:
	case symlinkPathParentsOnly:
		components = components[:len(components)-1]
	default:
		return fmt.Errorf("invalid symbolic-link path boundary %d", boundary)
	}
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
