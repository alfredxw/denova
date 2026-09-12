// Package agentprofiles commits settings-derived profile edits through the
// same Agents Project change service used by tools, editors and recovery.
package agentprofiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

// FileResult reports one independent commit. Paths are relative to the data
// directory and codes are stable; raw internal errors never enter the API DTO.
type FileResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Code   string `json:"code,omitempty"`
}

// MutationError retains every file outcome after partial success. Callers must
// refresh the canonical settings projection even when this error is returned.
type MutationError struct {
	Files  []FileResult
	causes []error
}

func (err *MutationError) Error() string {
	return "user settings files could not be saved: " + strings.Join(err.FailedPaths(), ", ")
}
func (err *MutationError) Unwrap() []error { return err.causes }
func (err *MutationError) FailedPaths() []string {
	var paths []string
	for _, file := range err.Files {
		if file.Status != "saved" {
			paths = append(paths, file.Path)
		}
	}
	return paths
}

// Mutate prepares file-local intents and commits each independently. A failed
// file never causes successful neighbours to be rolled back or overwritten.
func Mutate(ctx context.Context, dataDir string, request config.UserSettingsMutationRequest) (string, error) {
	plan, err := config.PlanUserSettingsMutation(dataDir, request)
	if err != nil {
		return "", err
	}
	if err := commit(ctx, dataDir, plan); err != nil {
		return "", err
	}
	return config.UserSettingsRevision(dataDir)
}

func commit(ctx context.Context, dataDir string, plan config.UserSettingsMutation) error {
	result := &MutationError{}
	record := func(path string, err error) {
		file := FileResult{Path: path, Status: "saved"}
		if err != nil {
			file.Status, file.Code = "failed", "save_failed"
			var changeErr *workspacechange.Error
			switch {
			case errors.Is(err, config.ErrInvalidAgentProfile):
				file.Code = "invalid_profile"
			case errors.Is(err, config.ErrSettingsRevisionConflict):
				file.Code = "revision_conflict"
			case errors.As(err, &changeErr):
				file.Code = changeErr.Code
				if changeErr.Code == workspacechange.ErrorCodeRevisionConflict {
					err = errors.Join(config.ErrSettingsRevisionConflict, err)
				}
			}
			result.causes = append(result.causes, err)
			slog.ErrorContext(ctx, "[agent-profiles] settings file commit failed", "path", path, "code", file.Code, "error", err)
		} else {
			slog.InfoContext(ctx, "[agent-profiles] settings file committed", "path", path)
		}
		result.Files = append(result.Files, file)
	}
	if plan.Config != nil {
		err := ctx.Err()
		if err == nil {
			err = config.WriteSettingsFileIfRevision(config.UserConfigPath(dataDir), plan.Config.Settings, plan.Config.BaseRevision)
		}
		record("config.toml", err)
	}
	var changes *workspacechange.Service
	var openErr error
	if len(plan.Profiles) > 0 {
		registry := project.NewRegistry(dataDir)
		record, err := registry.EnsureAgents(config.AgentProfilesRoot(dataDir))
		if err != nil {
			openErr = err
		} else {
			layout, err := registry.EnsureStore(record)
			if err != nil {
				openErr = err
			} else {
				changes, openErr = workspacechange.ForWorkspaceAt(layout.ContentRoot, layout.StoreRoot)
			}
		}
	}
	for _, profile := range plan.Profiles {
		err := profile.Err
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = openErr
		}
		if err == nil {
			metadata := workspacechange.ChangeMetadata{Origin: workspacechange.OriginUser, AutoAccept: true}
			switch profile.Operation {
			case config.AgentProfileReplace:
				_, err = changes.ReplaceFile(ctx, workspacechange.ReplaceFileRequest{Path: profile.Path, Content: string(profile.Content), BaseRevision: profile.BaseRevision, Metadata: metadata})
			case config.AgentProfileDelete:
				_, err = changes.DeleteFile(ctx, workspacechange.DeleteFileRequest{Path: profile.Path, BaseRevision: profile.BaseRevision, Metadata: metadata})
			default:
				err = fmt.Errorf("unsupported Agent Profile operation %q", profile.Operation)
			}
		}
		record("agents/"+profile.Path, err)
	}
	if len(result.causes) != 0 {
		return result
	}
	return nil
}
