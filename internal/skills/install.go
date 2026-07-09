package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// MaxInstallArchiveBytes bounds Skill ZIP uploads and GitHub archives.
	MaxInstallArchiveBytes int64 = 32 * 1024 * 1024

	maxInstallExtractedBytes int64 = 128 * 1024 * 1024
	maxInstallExtractedFiles       = 4096
)

var installSkillContainers = []string{
	"skills",
	"skills/.curated",
	"skills/.experimental",
	"skills/.system",
	".agents/skills",
	".codex/skills",
	".claude/skills",
	".aider-desk/skills",
	"data/skills",
	"agent/skills",
	".continue/skills",
	".cursor/skills",
	".goose/skills",
	".qwen/skills",
	".roo/skills",
}

// InstallCandidate describes one Skill directory discovered in an import source.
type InstallCandidate struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	SourcePath    string `json:"source_path"`
	Conflict      bool   `json:"conflict"`
	InvalidReason string `json:"invalid_reason,omitempty"`

	sourceDir string
}

// InstallPreview is returned before installation so the UI can let users select
// the specific Skills they want to install.
type InstallPreview struct {
	Candidates []InstallCandidate `json:"candidates"`
}

// InstallResult reports the Skills installed into a Denova scope.
type InstallResult struct {
	Installed []SkillSummary `json:"installed"`
}

func PreviewZip(ctx context.Context, dirs []Directory, scope Scope, data []byte) (InstallPreview, error) {
	return previewZip(ctx, dirs, scope, data, "")
}

func InstallZip(ctx context.Context, dirs []Directory, scope Scope, data []byte, candidateIDs []string) (InstallResult, error) {
	return installZip(ctx, dirs, scope, data, "", candidateIDs)
}

func previewZip(ctx context.Context, dirs []Directory, scope Scope, data []byte, subdir string) (InstallPreview, error) {
	root, cleanup, err := extractZipData(data)
	if err != nil {
		return InstallPreview{}, err
	}
	defer cleanup()
	searchRoot, err := zipSearchRoot(root, subdir)
	if err != nil {
		return InstallPreview{}, err
	}
	return PreviewDirectory(ctx, dirs, scope, searchRoot)
}

func installZip(ctx context.Context, dirs []Directory, scope Scope, data []byte, subdir string, candidateIDs []string) (InstallResult, error) {
	root, cleanup, err := extractZipData(data)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()
	searchRoot, err := zipSearchRoot(root, subdir)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallFromDirectory(ctx, dirs, scope, searchRoot, candidateIDs)
}

func PreviewDirectory(ctx context.Context, dirs []Directory, scope Scope, root string) (InstallPreview, error) {
	if ctx.Err() != nil {
		return InstallPreview{}, ctx.Err()
	}
	root = normalizePath(root)
	if root == "" {
		return InstallPreview{}, errors.New("skill source directory is required")
	}
	if fi, err := os.Stat(root); err != nil {
		return InstallPreview{}, err
	} else if !fi.IsDir() {
		return InstallPreview{}, fmt.Errorf("skill source is not a directory: %s", root)
	}

	skillDirs, err := discoverSkillDirs(root)
	if err != nil {
		return InstallPreview{}, err
	}
	candidates := make([]InstallCandidate, 0, len(skillDirs))
	for _, skillDir := range skillDirs {
		candidates = append(candidates, parseInstallCandidate(ctx, root, skillDir))
	}
	markCandidateNameConflicts(candidates)
	markTargetScopeConflicts(candidates, dirs, scope)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SourcePath < candidates[j].SourcePath
	})
	return InstallPreview{Candidates: candidates}, nil
}

func InstallFromDirectory(ctx context.Context, dirs []Directory, scope Scope, root string, candidateIDs []string) (InstallResult, error) {
	if ctx.Err() != nil {
		return InstallResult{}, ctx.Err()
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(dir.Path, 0o755); err != nil {
		return InstallResult{}, err
	}
	selected := normalizeCandidateIDSet(candidateIDs)
	if len(selected) == 0 {
		return InstallResult{}, errors.New("select at least one Skill to install")
	}
	preview, err := PreviewDirectory(ctx, dirs, scope, root)
	if err != nil {
		return InstallResult{}, err
	}
	byID := make(map[string]InstallCandidate, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		byID[candidate.ID] = candidate
	}

	candidates := make([]InstallCandidate, 0, len(selected))
	names := make(map[string]bool, len(selected))
	for id := range selected {
		candidate, ok := byID[id]
		if !ok {
			return InstallResult{}, fmt.Errorf("selected Skill candidate not found: %s", id)
		}
		if candidate.InvalidReason != "" {
			return InstallResult{}, fmt.Errorf("selected Skill is invalid: %s: %s", candidate.SourcePath, candidate.InvalidReason)
		}
		if candidate.Conflict {
			return InstallResult{}, fmt.Errorf("selected Skill conflicts with another Skill or target scope: %s", candidate.Name)
		}
		if names[candidate.Name] {
			return InstallResult{}, fmt.Errorf("selected Skill name is duplicated: %s", candidate.Name)
		}
		names[candidate.Name] = true
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	stageRoot, err := os.MkdirTemp(dir.Path, ".install-*")
	if err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(stageRoot)

	for _, candidate := range candidates {
		if err := copySkillDir(candidate.sourceDir, filepath.Join(stageRoot, candidate.Name)); err != nil {
			return InstallResult{}, err
		}
	}

	installedNames := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		target := filepath.Join(dir.Path, candidate.Name)
		if _, err := os.Stat(target); err == nil {
			for _, installed := range installedNames {
				_ = os.RemoveAll(filepath.Join(dir.Path, installed))
			}
			return InstallResult{}, fmt.Errorf("skill already exists in %s scope: %s", scope, candidate.Name)
		} else if err != nil && !os.IsNotExist(err) {
			for _, installed := range installedNames {
				_ = os.RemoveAll(filepath.Join(dir.Path, installed))
			}
			return InstallResult{}, err
		}
		if err := os.Rename(filepath.Join(stageRoot, candidate.Name), target); err != nil {
			for _, installed := range installedNames {
				_ = os.RemoveAll(filepath.Join(dir.Path, installed))
			}
			return InstallResult{}, err
		}
		installedNames = append(installedNames, candidate.Name)
	}

	installed := make([]SkillSummary, 0, len(installedNames))
	for _, name := range installedNames {
		doc, err := ReadDocument(ctx, dirs, scope, name)
		if err != nil {
			return InstallResult{}, err
		}
		installed = append(installed, doc.SkillSummary)
	}
	return InstallResult{Installed: installed}, nil
}

func extractZipData(data []byte) (string, func(), error) {
	if int64(len(data)) > MaxInstallArchiveBytes {
		return "", nil, fmt.Errorf("skill archive must be %dMB or smaller", MaxInstallArchiveBytes/(1024*1024))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil, fmt.Errorf("open Skill ZIP failed: %w", err)
	}
	root, err := os.MkdirTemp("", "denova-skill-install-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }

	var total int64
	files := 0
	for _, f := range reader.File {
		target, err := safeInstallJoin(root, f.Name)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		mode := f.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			cleanup()
			return "", nil, fmt.Errorf("Skill ZIP contains symlink: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				cleanup()
				return "", nil, err
			}
			continue
		}
		files++
		if files > maxInstallExtractedFiles {
			cleanup()
			return "", nil, fmt.Errorf("Skill ZIP contains too many files")
		}
		rc, err := f.Open()
		if err != nil {
			cleanup()
			return "", nil, err
		}
		written, writeErr := writeLimitedInstallFile(target, rc, mode.Perm(), maxInstallExtractedBytes-total)
		closeErr := rc.Close()
		if writeErr != nil {
			cleanup()
			return "", nil, writeErr
		}
		if closeErr != nil {
			cleanup()
			return "", nil, closeErr
		}
		total += written
		if total > maxInstallExtractedBytes {
			cleanup()
			return "", nil, fmt.Errorf("Skill ZIP extracted content is too large")
		}
	}
	return root, cleanup, nil
}

func zipSearchRoot(root, subdir string) (string, error) {
	searchRoot := archiveSearchRoot(root)
	subdir = strings.Trim(strings.TrimSpace(subdir), "/")
	if subdir == "" {
		return searchRoot, nil
	}
	target, err := safeInstallJoin(searchRoot, subdir)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(target); err != nil {
		return "", err
	} else if !fi.IsDir() {
		return "", fmt.Errorf("Skill archive subdir is not a directory: %s", subdir)
	}
	return target, nil
}

func archiveSearchRoot(root string) string {
	if fileExists(filepath.Join(root, SkillFileName)) || anyContainerExists(root) {
		return root
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return root
	}
	var onlyDir string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "__MACOSX") {
			continue
		}
		if !entry.IsDir() {
			return root
		}
		if onlyDir != "" {
			return root
		}
		onlyDir = filepath.Join(root, entry.Name())
	}
	if onlyDir == "" {
		return root
	}
	return onlyDir
}

func anyContainerExists(root string) bool {
	for _, container := range installSkillContainers {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(container))); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

func discoverSkillDirs(root string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(skillDir string) {
		skillDir = filepath.Clean(skillDir)
		if seen[skillDir] {
			return
		}
		seen[skillDir] = true
		out = append(out, skillDir)
	}
	if fileExists(filepath.Join(root, SkillFileName)) {
		add(root)
	}
	for _, container := range installSkillContainers {
		containerPath := filepath.Join(root, filepath.FromSlash(container))
		if fi, err := os.Stat(containerPath); err != nil || !fi.IsDir() {
			continue
		}
		if err := discoverContainerSkillDirs(containerPath, add); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func discoverContainerSkillDirs(containerPath string, add func(string)) error {
	entries, err := os.ReadDir(containerPath)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(containerPath, entry.Name())
		if fileExists(filepath.Join(child, SkillFileName)) {
			add(child)
			continue
		}
		grandchildren, err := os.ReadDir(child)
		if err != nil {
			return err
		}
		sort.Slice(grandchildren, func(i, j int) bool {
			return grandchildren[i].Name() < grandchildren[j].Name()
		})
		for _, grandchild := range grandchildren {
			if !grandchild.IsDir() {
				continue
			}
			grandchildPath := filepath.Join(child, grandchild.Name())
			if fileExists(filepath.Join(grandchildPath, SkillFileName)) {
				add(grandchildPath)
			}
		}
	}
	return nil
}

func parseInstallCandidate(ctx context.Context, root, skillDir string) InstallCandidate {
	sourcePath := relativeInstallSourcePath(root, skillDir)
	candidate := InstallCandidate{
		ID:         candidateID(sourcePath),
		SourcePath: sourcePath,
		sourceDir:  skillDir,
	}
	data, err := os.ReadFile(filepath.Join(skillDir, SkillFileName))
	if err != nil {
		candidate.InvalidReason = err.Error()
		return candidate
	}
	rec, err := parseRecord(ctx, Directory{Scope: ScopeUser, Path: root, Writable: false}, filepath.Join(skillDir, SkillFileName), string(data))
	if err != nil {
		candidate.InvalidReason = err.Error()
		return candidate
	}
	candidate.Name = rec.summary.Name
	candidate.Description = rec.summary.Description
	return candidate
}

func markCandidateNameConflicts(candidates []InstallCandidate) {
	counts := map[string]int{}
	for _, candidate := range candidates {
		if candidate.InvalidReason == "" && candidate.Name != "" {
			counts[candidate.Name]++
		}
	}
	for i := range candidates {
		if candidates[i].Name != "" && counts[candidates[i].Name] > 1 {
			candidates[i].Conflict = true
		}
	}
}

func markTargetScopeConflicts(candidates []InstallCandidate, dirs []Directory, scope Scope) {
	if scope == "" {
		return
	}
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return
	}
	for i := range candidates {
		if candidates[i].InvalidReason != "" || candidates[i].Name == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir.Path, candidates[i].Name)); err == nil {
			candidates[i].Conflict = true
		}
	}
}

func normalizeCandidateIDSet(candidateIDs []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range candidateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func copySkillDir(src, dst string) error {
	var total int64
	files := 0
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target, err := safeInstallJoin(dst, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Skill contains symlink: %s", rel)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Skill contains unsupported file: %s", rel)
		}
		files++
		if files > maxInstallExtractedFiles {
			return fmt.Errorf("Skill contains too many files")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		written, writeErr := writeLimitedInstallFile(target, in, info.Mode().Perm(), maxInstallExtractedBytes-total)
		closeErr := in.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		total += written
		if total > maxInstallExtractedBytes {
			return fmt.Errorf("Skill content is too large")
		}
		return nil
	})
}

func writeLimitedInstallFile(target string, reader io.Reader, mode os.FileMode, remaining int64) (int64, error) {
	if remaining < 0 {
		return 0, fmt.Errorf("Skill content is too large")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	limited := io.LimitReader(reader, remaining+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		return written, err
	}
	if written > remaining {
		return written, fmt.Errorf("Skill content is too large")
	}
	return written, nil
}

func safeInstallJoin(root, name string) (string, error) {
	root = filepath.Clean(root)
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if cleaned == "." || cleaned == "/" {
		return root, nil
	}
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("Skill archive contains invalid path: %s", name)
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Skill archive contains invalid path: %s", name)
	}
	return target, nil
}

func relativeInstallSourcePath(root, skillDir string) string {
	rel, err := filepath.Rel(root, skillDir)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func candidateID(sourcePath string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(sourcePath)))
	return hex.EncodeToString(sum[:8])
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
