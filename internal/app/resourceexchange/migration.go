package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agents/skills"
	"denova/internal/platform"
	"denova/internal/project"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

// MigrateSources runs before update workers. Each old source file is backed up
// and removed in the same durable transaction as its new installation record.
func (s *Service) MigrateSources(ctx context.Context) error {
	targets := []FileTarget{{Path: "skills"}}
	projects, err := s.registry.List(false)
	if err != nil {
		return err
	}
	for _, record := range projects {
		if record.Status == project.StatusAvailable {
			targets = append(targets, FileTarget{ProjectID: record.ID, Path: "skills"})
		}
	}
	for _, target := range targets {
		directory, err := resolveTarget(s.root, s.registry, target)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || skills.ValidateName(entry.Name()) != nil {
				continue
			}
			skillDir := filepath.Join(directory, entry.Name())
			state, clean, err := skills.LegacySourceState(ctx, skillDir)
			if err != nil {
				return err
			}
			if state == nil {
				continue
			}
			source, github, err := skills.CanonicalLegacySource(state.Source)
			if err != nil {
				return err
			}
			selectedPath := path.Join(source.Subdir, state.SourcePath)
			if selectedPath == "." {
				selectedPath = ""
			}
			origin := Source{Kind: "https_zip", URL: source.URL, Path: selectedPath}
			if github {
				origin.Kind = "github"
				origin.Ref = source.Ref
			}
			mode := "manual"
			if state.AutoUpdate {
				mode = "auto_apply"
			}
			local := LocalRef{Kind: "skill", Scope: "user", ID: entry.Name(), ProjectID: target.ProjectID}
			if target.ProjectID != "" {
				local.Scope = "workspace"
			}
			files, err := readFiles(skillDir)
			if err != nil {
				return err
			}
			delete(files, ".denova-source.json")
			baseline := map[string]string{}
			for name, raw := range files {
				digest := revisionfile.Revision(raw)
				if !clean {
					digest = "unknown:" + state.Digest
				}
				baseline[path.Join(target.Path, entry.Name(), name)] = digest
			}
			raw, err := archiveBytes(files)
			if err != nil {
				return err
			}
			sourceDigest := revisionfile.Revision(raw)
			if !clean {
				sourceDigest = "unknown:" + state.Digest
			}
			now := time.Now().UTC()
			item := Installation{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("denova-skill:"+target.ProjectID+":"+entry.Name())).String(), Package: PackageInfo{ID: "skill-" + entry.Name(), Name: entry.Name()}, Source: origin, ProjectID: target.ProjectID, Tracking: "tracked", UpdateMode: mode, CreatedAt: now, UpdatedAt: now, CheckedAt: state.CheckedAt, Bindings: []Binding{{ResourceID: "skill", Local: local, Ownership: "owned", SourceDigest: sourceDigest, Baseline: baseline}}}
			content, err := json.Marshal(item)
			if err != nil {
				return err
			}
			metadata := FileTarget{ProjectID: target.ProjectID, Path: path.Join(target.Path, entry.Name(), ".denova-source.json")}
			snapshot, err := s.snapshot(ctx, metadata)
			if err != nil {
				return err
			}
			if err := commitFiles(ctx, s.root, s.registry, uuid.NewString(), []fileChange{{Target: installationTarget(item.ID), Expected: revisionfile.MissingRevision, After: content}, {Target: metadata, Expected: snapshot.Revision, Delete: true}}); err != nil {
				return fmt.Errorf("migrate Skill source %s: %w", entry.Name(), err)
			}
		}
	}
	// Extension sources have not shipped in v0.5.0. Adopt current development
	// installs using the same record and remove their unpublished duplicate source.
	for _, kind := range []platform.Kind{platform.Plugin, platform.Game} {
		items, err := s.platform.List(kind)
		if err != nil {
			return err
		}
		for _, installed := range items {
			if installed.Removed || installed.Source == nil {
				continue
			}
			source := installed.Source
			if source.Path == "." {
				source.Path = ""
			}
			now := time.Now().UTC()
			item := Installation{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("denova-extension:"+string(kind)+":"+installed.ID)).String(), Package: PackageInfo{ID: "ext-" + strings.ReplaceAll(installed.ID, ".", "-"), Name: installed.ID}, Source: Source{Kind: "github", URL: source.URL, Ref: source.Ref, Path: source.Path, Commit: source.Commit}, Tracking: "tracked", UpdateMode: "manual", CreatedAt: now, UpdatedAt: now, Bindings: []Binding{{ResourceID: "extension", Local: LocalRef{Kind: "extension." + string(kind), Scope: "global", ID: installed.ID}, Ownership: "owned", SourceDigest: installed.CurrentRelease, Baseline: map[string]string{}}}}
			// Release digest is carried separately from the container's byte digest.
			for _, release := range installed.Releases {
				if release.Digest == installed.CurrentRelease {
					item.Package.Name = release.Manifest.Name.English
					item.Package.Version = release.Manifest.Version
				}
			}
			installed.Source = nil
			payload, err := json.Marshal(installed)
			if err != nil {
				return err
			}
			record, err := json.Marshal(item)
			if err != nil {
				return err
			}
			folder := "plugins"
			if kind == platform.Game {
				folder = "games"
			}
			target := FileTarget{Path: path.Join(folder, installed.ID, "installed.json")}
			before, err := s.snapshot(ctx, target)
			if err != nil {
				return err
			}
			if err := commitFiles(ctx, s.root, s.registry, uuid.NewString(), []fileChange{{Target: target, Expected: before.Revision, After: payload}, {Target: installationTarget(item.ID), Expected: revisionfile.MissingRevision, After: record}}); err != nil {
				return err
			}
		}
	}
	return nil
}
