package skills

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"denova/internal/portablepath"
)

// UpdateAction separates a manual check from permission to install changes.
type UpdateAction string

const (
	CheckUpdate UpdateAction = "check"
	ApplyUpdate UpdateAction = "update"
	autoUpdate  UpdateAction = "auto"
)

// UpdateResult is per Skill so a failed upstream never discards other results.
type UpdateResult struct {
	Scope    Scope        `json:"scope"`
	Name     string       `json:"name"`
	Remote   *RemoteState `json:"remote,omitempty"`
	ErrorKey string       `json:"error_key,omitempty"`
}

func RefreshRemote(ctx context.Context, dirs []Directory, scope Scope, name string, action UpdateAction) UpdateResult {
	return refreshRemote(ctx, dirs, scope, name, action, time.Now().UTC())
}

func refreshRemote(ctx context.Context, dirs []Directory, scope Scope, name string, action UpdateAction, now time.Time) UpdateResult {
	result := UpdateResult{Scope: scope, Name: name}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err == nil {
		err = ValidateName(name)
	}
	if err != nil {
		result.ErrorKey = "api.skills.updateFailed"
		return result
	}
	root := filepath.Join(dir.Path, name)
	state, err := readRemoteState(root)
	if err != nil || state == nil {
		result.ErrorKey = "api.skills.sourceRequired"
		return result
	}
	result.Remote = state
	if action == autoUpdate && (!state.AutoUpdate || (!state.CheckedAt.IsZero() && now.Sub(state.CheckedAt) < DefaultUpdateInterval)) {
		return result
	}
	if action != CheckUpdate && action != ApplyUpdate && action != autoUpdate {
		result.ErrorKey = "api.skills.updateFailed"
		return result
	}

	// Network I/O never holds the editing lease. Consent and source identity are
	// rechecked under the lease before installing or persisting check results.
	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	data, subdir, downloadErr := remoteArchiveData(fetchCtx, state.Source)
	var candidateRoot string
	cleanup := func() {}
	if downloadErr == nil {
		var extracted string
		extracted, cleanup, downloadErr = extractZipData(data)
		if downloadErr == nil {
			candidateRoot, downloadErr = zipSearchRoot(extracted, subdir)
			if downloadErr == nil {
				candidateRoot, downloadErr = safeInstallJoin(candidateRoot, state.SourcePath)
			}
		}
	}
	if cleanup != nil {
		defer cleanup()
	}
	updated, err := withSkillLease(ctx, dir, name, func() (*RemoteState, error) {
		current, err := readRemoteState(root)
		if err != nil {
			return current, err
		}
		if current == nil || current.Source != state.Source || current.SourcePath != state.SourcePath || current.Digest != state.Digest {
			return current, ErrRevisionConflict
		}
		if action == autoUpdate && (!current.AutoUpdate || (!current.CheckedAt.IsZero() && now.Sub(current.CheckedAt) < DefaultUpdateInterval)) {
			return current, nil
		}
		current.CheckedAt = now
		if downloadErr != nil {
			current.Status = "error"
			return current, errors.Join(downloadErr, writeRemoteState(root, current))
		}
		err = inspectRemoteCandidate(ctx, dir, name, root, candidateRoot, current, action)
		if err != nil {
			current.Status = "error"
		}
		return current, errors.Join(err, writeRemoteState(root, current))
	})
	if updated != nil {
		result.Remote = updated
	}
	if err != nil {
		slog.ErrorContext(ctx, "Refresh remote Skill failed", "scope", scope, "name", name, "action", action, "error", err)
		result.ErrorKey = "api.skills.updateFailed"
	} else {
		slog.InfoContext(ctx, "Remote Skill checked", "scope", scope, "name", name, "action", action, "status", result.Remote.Status)
	}
	return result
}

func inspectRemoteCandidate(ctx context.Context, dir Directory, name, root, candidateRoot string, state *RemoteState, action UpdateAction) error {
	if err := portablepath.PreflightTree(candidateRoot); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(candidateRoot, SkillFileName))
	if err != nil {
		return err
	}
	rec, err := parseRecord(ctx, dir, filepath.Join(candidateRoot, SkillFileName), string(data))
	if err != nil {
		return err
	}
	// A local rename must never be silently undone by the upstream name.
	if rec.skill.Name != name {
		state.Status = "modified"
		return nil
	}
	upstream, err := skillContentDigest(ctx, candidateRoot)
	if err != nil {
		return err
	}
	local, err := skillContentDigest(ctx, root)
	if err != nil {
		return err
	}
	state.UpdateAvailable = upstream != state.Digest
	if local != state.Digest {
		state.Status = "modified"
		return nil
	}
	state.Status = "current"
	if !state.UpdateAvailable {
		return nil
	}
	state.Status = "available"
	if action == CheckUpdate {
		return nil
	}
	staged, err := os.MkdirTemp(dir.Path, ".update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	if err := copySkillDir(candidateRoot, staged); err != nil {
		return err
	}
	next := *state
	next.Digest, next.Status, next.UpdateAvailable = upstream, "updated", false
	if err := writeRemoteState(staged, &next); err != nil {
		return err
	}
	if err := replaceRemoteSkill(dir, name, staged); err != nil {
		return err
	}
	*state = next
	return nil
}

// UpdateDue checks opted-in remote Skills once per day, including after a
// restart. Failures and local modifications also record the attempted time.
func UpdateDue(ctx context.Context, dirs []Directory, now time.Time) []UpdateResult {
	var results []UpdateResult
	for _, dir := range dirs {
		if !dir.Writable {
			continue
		}
		recoverDirectoryUpdates(ctx, dir)
		entries, err := os.ReadDir(dir.Path)
		if err != nil && !os.IsNotExist(err) {
			slog.ErrorContext(ctx, "Scan remote Skills for updates failed", "scope", dir.Scope, "error", err)
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return results
			}
			if !entry.IsDir() || ValidateName(entry.Name()) != nil {
				continue
			}
			state, err := readRemoteState(filepath.Join(dir.Path, entry.Name()))
			if err != nil {
				slog.ErrorContext(ctx, "Read remote Skill update state failed", "name", entry.Name(), "error", err)
				continue
			}
			if state == nil || !state.AutoUpdate || (!state.CheckedAt.IsZero() && now.Sub(state.CheckedAt) < DefaultUpdateInterval) {
				continue
			}
			results = append(results, refreshRemote(ctx, dirs, dir.Scope, entry.Name(), autoUpdate, now))
		}
	}
	return results
}

// The pending backup is a recovery marker as well as the previous directory.
// A crash between renames restores it when the library is next opened. After
// success it becomes a retained backup; all paths remain relative to the root.
func replaceRemoteSkill(dir Directory, name, staged string) error {
	backupRoot := filepath.Join(dir.Path, ".denova-backups", name)
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return err
	}
	if err := recoverSkillUpdate(dir, name); err != nil {
		return err
	}
	current := filepath.Join(dir.Path, name)
	pending := filepath.Join(backupRoot, "pending")
	if err := os.Rename(current, pending); err != nil {
		return err
	}
	if err := os.Rename(staged, current); err != nil {
		return errors.Join(err, os.Rename(pending, current))
	}
	if err := recoverSkillUpdate(dir, name); err != nil {
		// Replacement has committed. Keep its provenance; the pending backup
		// remains recoverable and will be finalized when the library reopens.
		slog.Error("Finalize Skill update backup failed", "name", name, "error", err)
	}
	return nil
}

func recoverSkillUpdate(dir Directory, name string) error {
	pending := filepath.Join(dir.Path, ".denova-backups", name, "pending")
	if _, err := os.Stat(pending); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	target := filepath.Join(dir.Path, name)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return os.Rename(pending, target)
	} else if err != nil {
		return err
	}
	return os.Rename(pending, filepath.Join(filepath.Dir(pending), fmt.Sprintf("%d", time.Now().UTC().UnixNano())))
}

func recoverDirectoryUpdates(ctx context.Context, dir Directory) {
	if !dir.Writable {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(dir.Path, ".denova-backups"))
	for _, entry := range entries {
		if !entry.IsDir() || ValidateName(entry.Name()) != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir.Path, ".denova-backups", entry.Name(), "pending")); err != nil {
			continue
		}
		_, err := withSkillLease(ctx, dir, entry.Name(), func() (struct{}, error) { return struct{}{}, recoverSkillUpdate(dir, entry.Name()) })
		if err != nil {
			slog.ErrorContext(ctx, "Recover interrupted Skill update failed", "name", entry.Name(), "error", err)
		}
	}
}
