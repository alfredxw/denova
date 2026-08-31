package projectfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/book"
	workspacechange "denova/internal/workspace/change"
)

type ReplaceRequest struct {
	Query       string
	Replacement string
	Regex       bool
}

type ReplaceResult struct {
	ProjectID         string                   `json:"project_id"`
	Workspace         string                   `json:"workspace"`
	Files             []book.ReplaceFileResult `json:"files"`
	TotalReplacements int                      `json:"total_replacements"`
	Skipped           []string                 `json:"skipped"`
}

func (service *Service) Search(ctx context.Context, projectID, query string, limit int, options book.SearchOptions) ([]book.SearchResult, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return nil, err
	}
	var results []book.SearchResult
	err = runtime.changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var searchErr error
		results, searchErr = book.SearchWorkspace(runtime.layout.ContentRoot, query, limit, options)
		return searchErr
	})
	return results, err
}

// Replace performs one Project-wide CAS replacement and creates a recoverable
// Book version before the first visible write. Concurrently edited files are
// skipped rather than overwritten.
func (service *Service) Replace(ctx context.Context, projectID string, request ReplaceRequest) (ReplaceResult, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return ReplaceResult{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return ReplaceResult{}, fmt.Errorf("replacement query is required")
	}
	replacer, err := book.NewReplacer(query, request.Replacement, book.SearchOptions{Regex: request.Regex})
	if err != nil {
		return ReplaceResult{}, err
	}
	result := ReplaceResult{
		ProjectID: runtime.record.ID, Workspace: runtime.layout.ContentRoot,
		Files: []book.ReplaceFileResult{}, Skipped: []string{},
	}
	var hasMatch bool
	err = runtime.changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var matchErr error
		hasMatch, matchErr = projectHasReplacement(runtime, replacer)
		return matchErr
	})
	if err != nil || !hasMatch {
		return result, err
	}
	if supportsVersions(runtime.record.Type) {
		versioning := service.mutationVersioning(runtime)
		if versioning.Service != nil {
			err = runtime.changes.WithConsistentWorkspaceSnapshot(ctx, func() error {
				_, createErr := versioning.Service.Create("Backup before replace all / 全局替换前自动备份", book.VersionSourceManual, versioning.Settings)
				if errors.Is(createErr, book.ErrVersionClean) {
					return nil
				}
				return createErr
			})
			if err != nil {
				return ReplaceResult{}, fmt.Errorf("create replacement backup: %w", err)
			}
		}
	}

	candidates, err := book.ListReplaceCandidateFiles(runtime.layout.ContentRoot, runtime.layout.StateRoot)
	if err != nil {
		return ReplaceResult{}, err
	}
	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return ReplaceResult{}, err
		}
		content, revision, readErr := runtime.changes.ReadFile(path)
		if readErr != nil || !book.IsSearchableContent([]byte(content)) {
			continue
		}
		next, count := replacer.ReplaceAll(content)
		if count == 0 || next == content {
			continue
		}
		saved, saveErr := runtime.changes.SaveFile(ctx, path, next, revision)
		if saveErr != nil {
			var changeErr *workspacechange.Error
			if errors.As(saveErr, &changeErr) && changeErr.Code == workspacechange.ErrorCodeRevisionConflict {
				slog.WarnContext(ctx, "[internal/app/projectfiles/search.go] skipped concurrently modified replacement target",
					"project_id", runtime.record.ID, "path", path,
				)
				result.Skipped = append(result.Skipped, path)
				continue
			}
			return ReplaceResult{}, saveErr
		}
		if !saved.Changed {
			continue
		}
		result.Files = append(result.Files, book.ReplaceFileResult{Path: path, Replacements: count})
		result.TotalReplacements += count
	}
	if len(result.Files) > 0 {
		versioning := service.mutationVersioning(runtime)
		if versioning.Service != nil {
			versioning.Service.ScheduleAutoVersion(versioning.Settings)
		}
	}
	return result, nil
}

func projectHasReplacement(runtime projectRuntime, replacer *book.Replacer) (bool, error) {
	candidates, err := book.ListReplaceCandidateFiles(runtime.layout.ContentRoot, runtime.layout.StateRoot)
	if err != nil {
		return false, err
	}
	for _, path := range candidates {
		data, readErr := os.ReadFile(filepath.Join(runtime.layout.ContentRoot, filepath.FromSlash(path)))
		if readErr != nil || !book.IsSearchableContent(data) {
			continue
		}
		if next, count := replacer.ReplaceAll(string(data)); count > 0 && next != string(data) {
			return true, nil
		}
	}
	return false, nil
}
