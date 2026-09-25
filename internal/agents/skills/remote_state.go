package skills

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"denova/internal/portablepath"
)

const remoteStateFile = ".denova-source.json"
const DefaultUpdateInterval = 24 * time.Hour

// RemoteState travels with an installed Skill. SourcePath is relative to the
// archive's selected subdirectory; Digest covers content, never mtimes or host
// paths. Availability and check results are distinct from automatic consent.
type RemoteState struct {
	Source          RemoteArchiveSource `json:"source"`
	SourcePath      string              `json:"source_path"`
	Digest          string              `json:"digest"`
	AutoUpdate      bool                `json:"auto_update"`
	CheckedAt       time.Time           `json:"checked_at"`
	UpdateAvailable bool                `json:"update_available"`
	Status          string              `json:"status,omitempty"`
}

func readRemoteState(root string) (*RemoteState, error) {
	data, err := os.ReadFile(filepath.Join(root, remoteStateFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state RemoteState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Source.URL == "" || state.Digest == "" || state.SourcePath == "" {
		return nil, fmt.Errorf("invalid remote Skill source metadata")
	}
	return &state, nil
}

func writeRemoteState(root string, state *RemoteState) error {
	if err := portablepath.CheckNoCollision(root, remoteStateFile); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return atomicWriteSkillFile(dir, remoteStateFile, data, 0o600)
}

func decorateRecord(ctx context.Context, rec *record) {
	if !rec.directory.Writable {
		return
	}
	state, err := readRemoteState(rec.skill.BaseDirectory)
	if err != nil {
		slog.ErrorContext(ctx, "Read remote Skill source failed", "name", rec.skill.Name, "error", err)
		return
	}
	rec.summary.Remote = state
}

// skillContentDigest ignores archive timestamps but includes every file name,
// executable bit and byte. Symlinks and oversized inputs fail closed.
func skillContentDigest(ctx context.Context, directory string) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	h := sha256.New()
	var total int64
	files := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == remoteStateFile || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Skill contains unsupported file: %s", name)
		}
		files++
		total += info.Size()
		if files > maxInstallExtractedFiles || total > maxInstallExtractedBytes {
			return fmt.Errorf("Skill content is too large")
		}
		writeSkillRevisionField(h, []byte(filepath.ToSlash(name)))
		writeSkillRevisionUint(h, uint64(info.Mode().Perm()&0o111))
		writeSkillRevisionUint(h, uint64(info.Size()))
		file, err := root.Open(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		written, err := io.Copy(h, io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if written != info.Size() {
			return fmt.Errorf("Skill changed while checking content")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// SetAutoUpdate records explicit user consent. Existing local/ZIP Skills have
// no recoverable upstream source and cannot opt into automatic replacement.
func SetAutoUpdate(ctx context.Context, dirs []Directory, scope Scope, name string, enabled bool) (RemoteState, error) {
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return RemoteState{}, err
	}
	return withSkillLease(ctx, dir, name, func() (RemoteState, error) {
		root := filepath.Join(dir.Path, name)
		state, err := readRemoteState(root)
		if err != nil {
			return RemoteState{}, err
		}
		if state == nil {
			return RemoteState{}, fmt.Errorf("Skill has no remote source")
		}
		state.AutoUpdate = enabled
		return *state, writeRemoteState(root, state)
	})
}

// LegacySourceState is used only to migrate the latest released Skill source
// metadata into application-owned installation records without resetting trust.
func LegacySourceState(ctx context.Context, directory string) (*RemoteState, bool, error) {
	state, err := readRemoteState(directory)
	if err != nil || state == nil {
		return state, false, err
	}
	digest, err := skillContentDigest(ctx, directory)
	return state, digest == state.Digest, err
}

// CanonicalLegacySource resolves historical GitHub shorthand and tree URLs
// without downloading. Non-GitHub archives retain their original URL/subdir.
func CanonicalLegacySource(source RemoteArchiveSource) (RemoteArchiveSource, bool, error) {
	repo, github, err := githubRepositoryFromRemoteSource(source)
	if err != nil {
		return source, github, err
	}
	if github {
		return RemoteArchiveSource{URL: "https://github.com/" + repo.Owner + "/" + repo.Repo, Ref: repo.Ref, Subdir: repo.Subdir}, true, nil
	}
	return source, false, nil
}
