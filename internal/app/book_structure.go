package app

import (
	"context"
	"errors"
	"log/slog"

	"denova/internal/book"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

// BookStructure resolves by stable Project ID, never by the foreground book.
// A nonempty revision applies exactly the previewed source under the same
// mutation lease used by editor saves, Agent writes and version restoration.
func (a *App) BookStructure(ctx context.Context, projectID string, target book.ChapterStructure, revision string, titles book.StructureTitles, backupMessage string) (book.StructurePreview, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return book.StructurePreview{}, err
	}
	defer operation.Release()
	if operation.Layout().Type != projectdomain.TypeBook {
		return book.StructurePreview{}, errors.New("book project required")
	}
	resources, err := a.ProjectFiles().ProjectVersions(projectID)
	if err != nil {
		return book.StructurePreview{}, err
	}
	changes, err := workspacechange.ForWorkspaceAt(resources.Workspace, resources.StateRoot)
	if err != nil {
		return book.StructurePreview{}, err
	}
	var preview book.StructurePreview
	action := func() error {
		var err error
		preview, err = resources.Files.PreviewStructure(target, titles)
		if err != nil || revision == "" {
			return err
		}
		if revision != preview.Revision {
			return book.ErrStructureConflict
		}
		if target == preview.From {
			return nil
		}
		if err := resources.Files.PreserveEmptyManuscriptDirectories(preview.Before); err != nil {
			return err
		}
		if _, err := resources.VersionService.Create(backupMessage, book.VersionSourceRollbackBackup, resources.Settings); err != nil {
			return err
		}
		return resources.Files.ApplyStructure(preview)
	}
	if revision == "" {
		err = changes.WithConsistentWorkspaceSnapshot(operation.Context(), action)
	} else {
		err = changes.WithExclusiveWorkspace(operation.Context(), action)
		if err == nil {
			a.ProjectBook().InvalidateSummary(projectID, nil, true)
			resources.VersionService.ScheduleAutoVersion(resources.Settings)
			slog.InfoContext(ctx, "Changed book chapter structure", "project_id", projectID, "from", preview.From, "to", target)
		}
	}
	return preview, err
}
