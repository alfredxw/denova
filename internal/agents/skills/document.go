package skills

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSkillFileBytes int64 = 512 * 1024

func ReadDocument(ctx context.Context, dirs []Directory, scope Scope, name string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	if dir.Writable {
		return withSkillLease(ctx, dir, name, func() (Document, error) {
			return readDocumentLocked(ctx, dirs, dir, name)
		})
	}
	return readDocumentLocked(ctx, dirs, dir, name)
}

func readDocumentLocked(ctx context.Context, dirs []Directory, dir Directory, name string) (Document, error) {
	dirs = dedupeDirectories(dirs)
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return Document{}, err
	}
	defer skillRoot.Close()
	data, err := skillRoot.ReadFile(SkillFileName)
	if err != nil {
		return Document{}, err
	}
	path := filepath.Join(dir.Path, name, SkillFileName)
	rec, err := parseRecord(ctx, dir, path, string(data))
	if err != nil {
		return Document{}, err
	}
	active := activeRecordKeys(loadRecords(ctx, dirs))
	rec.summary.Active = active[recordKey(rec)]
	files, err := listSkillFilesFromRoot(ctx, skillRoot, dir.Writable)
	if err != nil {
		return Document{}, err
	}
	revision, err := skillDirectoryRevision(ctx, skillRoot)
	if err != nil {
		return Document{}, err
	}
	return Document{SkillSummary: rec.summary, Content: string(data), Revision: revision, Files: files}, nil
}

func CreateDocument(ctx context.Context, dirs []Directory, scope Scope, name, description string, agents ...string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	content := DefaultContent(name, description, agents...)
	return withSkillLease(ctx, dir, name, func() (Document, error) {
		return writeDocumentLocked(ctx, dirs, dir, name, content, false)
	})
}

// CreateDocumentWithContent creates a Skill from a complete SKILL.md without
// ever overwriting an existing directory.
func CreateDocumentWithContent(ctx context.Context, dirs []Directory, scope Scope, name, content string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	return withSkillLease(ctx, dir, name, func() (Document, error) {
		return writeDocumentLocked(ctx, dirs, dir, name, content, false)
	})
}

func SaveDocument(ctx context.Context, dirs []Directory, scope Scope, name, content string) (Document, error) {
	return SaveDocumentIfRevision(ctx, dirs, scope, name, content, "")
}

// SaveDocumentIfRevision updates SKILL.md only when the complete Skill
// directory still matches baseRevision. Supporting-file changes therefore
// invalidate a stale root edit instead of being silently ignored.
func SaveDocumentIfRevision(ctx context.Context, dirs []Directory, scope Scope, name, content, baseRevision string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	return withSkillLease(ctx, dir, name, func() (Document, error) {
		if err := validateSkillDirectoryRevision(ctx, dir, name, baseRevision, false); err != nil {
			return Document{}, err
		}
		return writeDocumentLocked(ctx, dirs, dir, name, content, true)
	})
}

// SaveDocumentAs writes a skill directory to a new editable scope/name. Editable
// sources are moved after the new copy has been validated and written; read-only
// sources are copied so built-in Skills can be overridden without losing files.
func SaveDocumentAs(ctx context.Context, dirs []Directory, sourceScope Scope, sourceName string, targetScope Scope, targetName, content string) (Document, error) {
	return SaveDocumentAsIfRevision(ctx, dirs, sourceScope, sourceName, targetScope, targetName, content, "")
}

// SaveDocumentAsIfRevision moves or copies a Skill only when the source entry still matches baseRevision.
func SaveDocumentAsIfRevision(ctx context.Context, dirs []Directory, sourceScope Scope, sourceName string, targetScope Scope, targetName, content, baseRevision string) (Document, error) {
	sourceName = strings.TrimSpace(sourceName)
	targetName = strings.TrimSpace(targetName)
	if targetScope == "" {
		targetScope = sourceScope
	}
	if targetName == "" {
		targetName = sourceName
	}
	if sourceScope == targetScope && sourceName == targetName {
		return SaveDocumentIfRevision(ctx, dirs, sourceScope, sourceName, content, baseRevision)
	}
	if err := ValidateName(sourceName); err != nil {
		return Document{}, err
	}
	if err := ValidateName(targetName); err != nil {
		return Document{}, err
	}
	sourceDir, err := directoryForScope(dirs, sourceScope)
	if err != nil {
		return Document{}, err
	}
	targetDir, err := writableDirectoryForScope(dirs, targetScope)
	if err != nil {
		return Document{}, err
	}
	targets := []skillLeaseTarget{{dir: targetDir, name: targetName}}
	if sourceDir.Writable {
		targets = append(targets, skillLeaseTarget{dir: sourceDir, name: sourceName})
	}
	return withSkillLeases(ctx, targets, func() (Document, error) {
		if err := validateSkillDirectoryRevision(ctx, sourceDir, sourceName, baseRevision, false); err != nil {
			return Document{}, err
		}
		return saveDocumentAsLocked(ctx, dirs, sourceDir, sourceName, targetDir, targetName, content)
	})
}

func saveDocumentAsLocked(ctx context.Context, dirs []Directory, sourceDir Directory, sourceName string, targetDir Directory, targetName, content string) (Document, error) {
	sourceSkillDir := filepath.Join(sourceDir.Path, sourceName)
	if _, err := os.Stat(filepath.Join(sourceSkillDir, SkillFileName)); err != nil {
		return Document{}, err
	}
	targetSkillDir := filepath.Join(targetDir.Path, targetName)
	if _, err := os.Stat(targetSkillDir); err == nil {
		return Document{}, fmt.Errorf("skill already exists in %s scope: %s", targetDir.Scope, targetName)
	} else if !os.IsNotExist(err) {
		return Document{}, err
	}
	targetPath := filepath.Join(targetSkillDir, SkillFileName)
	rec, err := parseRecord(ctx, targetDir, targetPath, content)
	if err != nil {
		return Document{}, err
	}
	if rec.skill.Name != targetName {
		return Document{}, fmt.Errorf("frontmatter name %q must match skill directory %q", rec.skill.Name, targetName)
	}
	if err := os.MkdirAll(targetDir.Path, 0o755); err != nil {
		return Document{}, err
	}
	stageRoot, err := os.MkdirTemp(targetDir.Path, ".save-*")
	if err != nil {
		return Document{}, err
	}
	defer os.RemoveAll(stageRoot)
	stageSkillDir := filepath.Join(stageRoot, targetName)
	if err := copySkillDir(sourceSkillDir, stageSkillDir); err != nil {
		return Document{}, err
	}
	if err := os.WriteFile(filepath.Join(stageSkillDir, SkillFileName), []byte(content), 0o644); err != nil {
		return Document{}, err
	}
	if err := os.Rename(stageSkillDir, targetSkillDir); err != nil {
		return Document{}, err
	}
	if sourceDir.Writable {
		if err := os.RemoveAll(sourceSkillDir); err != nil {
			return Document{}, err
		}
	}
	return readDocumentLocked(ctx, dirs, targetDir, targetName)
}

func ListSkillFiles(ctx context.Context, dirs []Directory, scope Scope, name string) ([]SkillFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return nil, err
	}
	if dir.Writable {
		return withSkillLease(ctx, dir, name, func() ([]SkillFile, error) {
			return listSkillFilesLocked(ctx, dir, name)
		})
	}
	return listSkillFilesLocked(ctx, dir, name)
}

func listSkillFilesLocked(ctx context.Context, dir Directory, name string) ([]SkillFile, error) {
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return nil, err
	}
	defer skillRoot.Close()
	return listSkillFilesFromRoot(ctx, skillRoot, dir.Writable)
}

func listSkillFilesFromRoot(ctx context.Context, skillRoot *os.Root, writable bool) ([]SkillFile, error) {
	var files []SkillFile
	if err := fs.WalkDir(skillRoot.FS(), ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel := path.Clean(filepath.ToSlash(filePath))
		files = append(files, skillFileFromInfo(rel, info, writable))
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Entry != files[j].Entry {
			return files[i].Entry
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func ReadSkillFile(ctx context.Context, dirs []Directory, scope Scope, name, filePath string) (FileDocument, error) {
	if err := ctx.Err(); err != nil {
		return FileDocument{}, err
	}
	if err := ValidateName(name); err != nil {
		return FileDocument{}, err
	}
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return FileDocument{}, err
	}
	if dir.Writable {
		return withSkillLease(ctx, dir, name, func() (FileDocument, error) {
			return readSkillFileLocked(ctx, dirs, dir, name, filePath)
		})
	}
	return readSkillFileLocked(ctx, dirs, dir, name, filePath)
}

func readSkillFileLocked(ctx context.Context, dirs []Directory, dir Directory, name, filePath string) (FileDocument, error) {
	skillDir := filepath.Join(dir.Path, name)
	rel, _, err := safeSkillFilePath(skillDir, filePath)
	if err != nil {
		return FileDocument{}, err
	}
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return FileDocument{}, err
	}
	defer skillRoot.Close()
	file, err := skillRoot.Open(filepath.FromSlash(rel))
	if err != nil {
		return FileDocument{}, err
	}
	defer file.Close()
	info, err := regularSkillFileInfoFromFile(file, rel)
	if err != nil {
		return FileDocument{}, err
	}
	if info.Size() > maxSkillFileBytes {
		return FileDocument{}, fmt.Errorf("skill file is too large to open: %s", rel)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSkillFileBytes+1))
	if err != nil {
		return FileDocument{}, err
	}
	if int64(len(data)) > maxSkillFileBytes {
		return FileDocument{}, fmt.Errorf("skill file is too large to open: %s", rel)
	}
	if !utf8.Valid(data) {
		return FileDocument{}, fmt.Errorf("skill file is not valid UTF-8 text: %s", rel)
	}
	doc, err := readDocumentLocked(ctx, dirs, dir, name)
	if err != nil {
		return FileDocument{}, err
	}
	return FileDocument{
		Skill:    doc.SkillSummary,
		File:     skillFileFromInfo(rel, info, dir.Writable),
		Content:  string(data),
		Revision: skillContentRevision(data),
	}, nil
}

func SaveSkillFile(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content string) (FileDocument, error) {
	return SaveSkillFileIfRevision(ctx, dirs, scope, name, filePath, content, "")
}

// CreateSkillFile creates one UTF-8 supporting file without overwriting an
// existing path. Parent directories are created beneath the scoped Skill root.
func CreateSkillFile(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content string) (FileDocument, error) {
	if err := ValidateName(name); err != nil {
		return FileDocument{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return FileDocument{}, err
	}
	return withSkillLease(ctx, dir, name, func() (FileDocument, error) {
		return createSkillFileLocked(ctx, dirs, scope, name, filePath, content)
	})
}

func createSkillFileLocked(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content string) (FileDocument, error) {
	if err := ctx.Err(); err != nil {
		return FileDocument{}, err
	}
	skillDir, dir, err := writableSkillDirectory(dirs, scope, name)
	if err != nil {
		return FileDocument{}, err
	}
	rel, _, err := safeSkillFilePath(skillDir, filePath)
	if err != nil {
		return FileDocument{}, err
	}
	if rel == SkillFileName {
		return FileDocument{}, fmt.Errorf("use CreateDocument to create %s", SkillFileName)
	}
	if int64(len([]byte(content))) > maxSkillFileBytes {
		return FileDocument{}, fmt.Errorf("skill file is too large to save: %s", rel)
	}
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return FileDocument{}, err
	}
	defer skillRoot.Close()
	if _, statErr := skillRoot.Lstat(filepath.FromSlash(rel)); statErr == nil {
		return FileDocument{}, fmt.Errorf("skill file already exists: %s", rel)
	} else if !os.IsNotExist(statErr) {
		return FileDocument{}, statErr
	}
	parent := filepath.FromSlash(path.Dir(rel))
	if parent != "." {
		if err := skillRoot.MkdirAll(parent, 0o755); err != nil {
			return FileDocument{}, err
		}
	}
	if err := atomicCreateSkillFile(skillRoot, rel, []byte(content), 0o644); err != nil {
		return FileDocument{}, err
	}
	return readSavedSkillFile(ctx, dirs, scope, name, rel)
}

// SaveSkillFileIfRevision updates a supporting file only when its exact content still matches baseRevision.
func SaveSkillFileIfRevision(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content, baseRevision string) (FileDocument, error) {
	if err := ValidateName(name); err != nil {
		return FileDocument{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return FileDocument{}, err
	}
	return withSkillLease(ctx, dir, name, func() (FileDocument, error) {
		return saveSkillFileIfRevisionLocked(ctx, dirs, scope, name, filePath, content, baseRevision)
	})
}

func saveSkillFileIfRevisionLocked(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content, baseRevision string) (FileDocument, error) {
	if ctx.Err() != nil {
		return FileDocument{}, ctx.Err()
	}
	skillDir, dir, err := writableSkillDirectory(dirs, scope, name)
	if err != nil {
		return FileDocument{}, err
	}
	rel, _, err := safeSkillFilePath(skillDir, filePath)
	if err != nil {
		return FileDocument{}, err
	}
	if rel == SkillFileName {
		return FileDocument{}, fmt.Errorf("use SaveDocument to update %s", SkillFileName)
	}
	if int64(len([]byte(content))) > maxSkillFileBytes {
		return FileDocument{}, fmt.Errorf("skill file is too large to save: %s", rel)
	}
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return FileDocument{}, err
	}
	defer skillRoot.Close()
	info, err := skillRoot.Lstat(filepath.FromSlash(rel))
	if err != nil {
		return FileDocument{}, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return FileDocument{}, fmt.Errorf("skill path is not a regular file: %s", rel)
	}
	if baseRevision != "" {
		current, err := skillRoot.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return FileDocument{}, err
		}
		if skillContentRevision(current) != baseRevision {
			return FileDocument{}, ErrRevisionConflict
		}
	}
	if err := atomicWriteSkillFile(skillRoot, rel, []byte(content), info.Mode().Perm()); err != nil {
		return FileDocument{}, err
	}
	return readSavedSkillFile(ctx, dirs, scope, name, rel)
}

// DeleteSkillFileIfRevision removes one supporting file only when its exact
// pre-delete content still matches baseRevision.
func DeleteSkillFileIfRevision(ctx context.Context, dirs []Directory, scope Scope, name, filePath, baseRevision string) (DeletedFile, error) {
	if err := ValidateName(name); err != nil {
		return DeletedFile{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return DeletedFile{}, err
	}
	return withSkillLease(ctx, dir, name, func() (DeletedFile, error) {
		return deleteSkillFileIfRevisionLocked(ctx, dirs, scope, name, filePath, baseRevision)
	})
}

func deleteSkillFileIfRevisionLocked(ctx context.Context, dirs []Directory, scope Scope, name, filePath, baseRevision string) (DeletedFile, error) {
	if err := ctx.Err(); err != nil {
		return DeletedFile{}, err
	}
	skillDir, dir, err := writableSkillDirectory(dirs, scope, name)
	if err != nil {
		return DeletedFile{}, err
	}
	rel, _, err := safeSkillFilePath(skillDir, filePath)
	if err != nil {
		return DeletedFile{}, err
	}
	if rel == SkillFileName {
		return DeletedFile{}, fmt.Errorf("use DeleteDocument to remove %s", SkillFileName)
	}
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return DeletedFile{}, err
	}
	defer skillRoot.Close()
	data, err := skillRoot.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		return DeletedFile{}, err
	}
	revision := skillContentRevision(data)
	if strings.TrimSpace(baseRevision) == "" || revision != strings.TrimSpace(baseRevision) {
		return DeletedFile{}, ErrRevisionConflict
	}
	info, err := skillRoot.Lstat(filepath.FromSlash(rel))
	if err != nil {
		return DeletedFile{}, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return DeletedFile{}, fmt.Errorf("skill path is not a regular file: %s", rel)
	}
	if err := skillRoot.Remove(filepath.FromSlash(rel)); err != nil {
		return DeletedFile{}, err
	}
	doc, err := readDocumentLocked(ctx, dirs, dir, name)
	if err != nil {
		return DeletedFile{}, err
	}
	return DeletedFile{Skill: doc.SkillSummary, Path: rel, Revision: revision}, nil
}

func readSavedSkillFile(ctx context.Context, dirs []Directory, scope Scope, name, rel string) (FileDocument, error) {
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return FileDocument{}, err
	}
	return readSkillFileLocked(ctx, dirs, dir, name, rel)
}

func DeleteDocument(ctx context.Context, dirs []Directory, scope Scope, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return err
	}
	_, err = withSkillLease(ctx, dir, name, func() (struct{}, error) {
		return struct{}{}, os.RemoveAll(filepath.Join(dir.Path, name))
	})
	return err
}

// DeleteDocumentIfRevision removes the complete Skill directory only when its
// directory-wide revision still matches the caller's snapshot. The returned
// document is the exact pre-delete state used for the comparison.
func DeleteDocumentIfRevision(ctx context.Context, dirs []Directory, scope Scope, name, baseRevision string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	return withSkillLease(ctx, dir, name, func() (Document, error) {
		doc, err := readDocumentLocked(ctx, dirs, dir, name)
		if err != nil {
			return Document{}, err
		}
		if strings.TrimSpace(baseRevision) == "" || doc.Revision != strings.TrimSpace(baseRevision) {
			return Document{}, ErrRevisionConflict
		}
		if err := os.RemoveAll(filepath.Join(dir.Path, name)); err != nil {
			return Document{}, err
		}
		return doc, nil
	})
}

func DefaultContent(name, description string, agents ...string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		description = fmt.Sprintf("Use this skill when the user asks for %s-specific guidance.", name)
	}
	frontmatter := marshalFrontmatter(name, description, normalizeAgentList(agents))
	return fmt.Sprintf(`---
%s---

# %s

Describe when to use this skill, what context to gather, and the concrete workflow the agent should follow.
`, frontmatter, name)
}

func writeDocumentLocked(ctx context.Context, dirs []Directory, dir Directory, name, content string, overwrite bool) (Document, error) {
	if ctx.Err() != nil {
		return Document{}, ctx.Err()
	}
	skillDir := filepath.Join(dir.Path, name)
	documentPath := filepath.Join(skillDir, SkillFileName)
	rec, err := parseRecord(ctx, dir, documentPath, content)
	if err != nil {
		return Document{}, err
	}
	if rec.skill.Name != name {
		return Document{}, fmt.Errorf("frontmatter name %q must match skill directory %q", rec.skill.Name, name)
	}
	if err := os.MkdirAll(dir.Path, 0o755); err != nil {
		return Document{}, err
	}
	scopeRoot, err := os.OpenRoot(dir.Path)
	if err != nil {
		return Document{}, err
	}
	defer scopeRoot.Close()

	created := false
	if _, statErr := scopeRoot.Lstat(name); statErr == nil {
		if !overwrite {
			return Document{}, fmt.Errorf("skill already exists: %s", name)
		}
	} else if os.IsNotExist(statErr) {
		if err := scopeRoot.Mkdir(name, 0o755); err != nil {
			return Document{}, err
		}
		created = true
	} else {
		return Document{}, statErr
	}

	var skillRoot *os.Root
	if created {
		skillRoot, err = scopeRoot.OpenRoot(name)
	} else {
		skillRoot, err = openScopedSkillRoot(dir, name)
	}
	if err != nil {
		if created {
			_ = scopeRoot.Remove(name)
		}
		return Document{}, fmt.Errorf("open skill %q in %s scope: %w", name, dir.Scope, err)
	}
	defer skillRoot.Close()
	if err := atomicWriteSkillFile(skillRoot, SkillFileName, []byte(content), 0o644); err != nil {
		if created {
			_ = scopeRoot.Remove(name)
		}
		return Document{}, err
	}
	doc, err := readDocumentLocked(ctx, dirs, dir, name)
	if err != nil {
		return Document{}, err
	}
	return doc, nil
}

func skillDirectory(dirs []Directory, scope Scope, name string) (string, Directory, error) {
	if err := ValidateName(name); err != nil {
		return "", Directory{}, err
	}
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return "", Directory{}, err
	}
	skillDir := filepath.Join(dir.Path, name)
	if _, err := os.Stat(filepath.Join(skillDir, SkillFileName)); err != nil {
		return "", Directory{}, err
	}
	return skillDir, dir, nil
}

func writableSkillDirectory(dirs []Directory, scope Scope, name string) (string, Directory, error) {
	if err := ValidateName(name); err != nil {
		return "", Directory{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return "", Directory{}, err
	}
	skillDir := filepath.Join(dir.Path, name)
	if _, err := os.Stat(filepath.Join(skillDir, SkillFileName)); err != nil {
		return "", Directory{}, err
	}
	return skillDir, dir, nil
}

func safeSkillFilePath(skillDir, filePath string) (string, string, error) {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/"))
	if cleaned == "." || cleaned == "/" || cleaned == "" {
		return "", "", fmt.Errorf("skill file path is required")
	}
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", "", fmt.Errorf("invalid skill file path: %s", filePath)
	}
	abs := filepath.Join(skillDir, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(skillDir, abs)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("invalid skill file path: %s", filePath)
	}
	return filepath.ToSlash(rel), abs, nil
}

func regularSkillFileInfo(filePath string) (os.FileInfo, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill path is not a regular file: %s", filePath)
	}
	return info, nil
}

func regularSkillFileInfoFromFile(file *os.File, displayPath string) (os.FileInfo, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill path is not a regular file: %s", displayPath)
	}
	return info, nil
}

func openScopedSkillRoot(dir Directory, name string) (*os.Root, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	scopeRoot, err := os.OpenRoot(dir.Path)
	if err != nil {
		return nil, err
	}
	defer scopeRoot.Close()
	skillRoot, err := scopeRoot.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("skill directory escapes its %s scope: %w", dir.Scope, err)
	}
	info, err := skillRoot.Lstat(SkillFileName)
	if err != nil {
		skillRoot.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		skillRoot.Close()
		return nil, fmt.Errorf("skill entry is not a regular file: %s", SkillFileName)
	}
	return skillRoot, nil
}

func skillFileFromInfo(rel string, info os.FileInfo, writable bool) SkillFile {
	return SkillFile{
		Path:      rel,
		Size:      info.Size(),
		Entry:     rel == SkillFileName,
		Editable:  writable && info.Size() <= maxSkillFileBytes,
		UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
	}
}
