package app

import (
	"context"
	"fmt"
	"strings"

	projectdomain "denova/internal/project"
)

type ProjectRecord = projectdomain.Record
type ProjectType = projectdomain.Type
type ProjectStatus = projectdomain.Status
type ProjectLayout = projectdomain.Layout

const (
	ProjectTypeBook    = projectdomain.TypeBook
	ProjectTypeGeneral = projectdomain.TypeGeneral

	ProjectStatusAvailable = projectdomain.StatusAvailable
	ProjectStatusMissing   = projectdomain.StatusMissing
	ProjectStatusArchived  = projectdomain.StatusArchived
)

// Projects lists user-managed Project definitions. Missing directories remain
// visible so the user can relink them without losing conversations or config.
func (a *App) Projects(includeArchived bool) ([]ProjectRecord, error) {
	if a == nil || a.projectRegistry == nil {
		return []ProjectRecord{}, nil
	}
	return a.projectRegistry.List(includeArchived)
}

// AddProject registers a directory with a folder-derived display name. New
// directories are classified as Book or General from their content markers;
// re-adding an existing Project preserves its durable type and custom name.
func (a *App) AddProject(path string) (ProjectRecord, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, fmt.Errorf("project registry is unavailable")
	}
	kind, err := projectdomain.DetectType(path)
	if err != nil {
		return ProjectRecord{}, err
	}
	if existing, found, findErr := a.projectRegistry.FindByPath(path, true); findErr != nil {
		return ProjectRecord{}, findErr
	} else if found {
		kind = existing.Type
	}
	record, err := a.projectRegistry.Add(path, kind, "")
	if err != nil {
		return ProjectRecord{}, err
	}
	if _, err := a.projectRegistry.EnsureState(record); err != nil {
		return ProjectRecord{}, err
	}
	return record, nil
}

func (a *App) RenameProject(id, name string) (ProjectRecord, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, fmt.Errorf("project registry is unavailable")
	}
	return a.projectRegistry.Rename(id, name)
}

func (a *App) ArchiveProject(id string) (ProjectRecord, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, fmt.Errorf("project registry is unavailable")
	}
	if a.agentChatApp != nil {
		if err := a.agentChatApp.closeProject(context.Background(), id); err != nil {
			return ProjectRecord{}, err
		}
	}
	return a.projectRegistry.Archive(id)
}

func (a *App) RelinkProject(id, path string) (ProjectRecord, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, fmt.Errorf("project registry is unavailable")
	}
	if a.agentChatApp != nil {
		if err := a.agentChatApp.closeProject(context.Background(), id); err != nil {
			return ProjectRecord{}, err
		}
	}
	record, err := a.projectRegistry.Relink(id, path)
	if err != nil {
		return ProjectRecord{}, err
	}
	if _, err := a.projectRegistry.EnsureState(record); err != nil {
		return ProjectRecord{}, err
	}
	return record, nil
}

func (a *App) ReorderProjects(ids []string) error {
	if a == nil || a.projectRegistry == nil {
		return fmt.Errorf("project registry is unavailable")
	}
	return a.projectRegistry.Reorder(ids)
}

func (a *App) resolveProject(id string, requireAvailable bool) (ProjectRecord, ProjectLayout, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, ProjectLayout{}, fmt.Errorf("project registry is unavailable")
	}
	id = strings.TrimSpace(id)
	record, err := a.projectRegistry.Get(id)
	if err != nil {
		return ProjectRecord{}, ProjectLayout{}, err
	}
	if record.Status == ProjectStatusArchived {
		return ProjectRecord{}, ProjectLayout{}, fmt.Errorf("project %s is archived", id)
	}
	if requireAvailable && record.Status != ProjectStatusAvailable {
		return ProjectRecord{}, ProjectLayout{}, fmt.Errorf("project directory is unavailable: %s", record.WorkspacePath)
	}
	layout, err := a.projectRegistry.EnsureState(record)
	if err != nil {
		return ProjectRecord{}, ProjectLayout{}, err
	}
	return record, layout, nil
}

func (a *App) resolveProjectByWorkspace(workspace string) (ProjectRecord, ProjectLayout, error) {
	if a == nil || a.projectRegistry == nil {
		return ProjectRecord{}, ProjectLayout{}, fmt.Errorf("project registry is unavailable")
	}
	record, found, err := a.projectRegistry.FindByPath(workspace, false)
	if err != nil {
		return ProjectRecord{}, ProjectLayout{}, err
	}
	if !found {
		return ProjectRecord{}, ProjectLayout{}, fmt.Errorf("directory is not a registered project")
	}
	return a.resolveProject(record.ID, true)
}

func (a *App) projectLayoutForWorkspace(workspace string) (ProjectLayout, error) {
	_, layout, err := a.resolveProjectByWorkspace(workspace)
	return layout, err
}
