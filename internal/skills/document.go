package skills

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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
	files, err := ListSkillFiles(ctx, dirs, scope, name)
	if err != nil {
		return Document{}, err
	}
	return Document{SkillSummary: rec.summary, Content: string(data), Files: files}, nil
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
	return writeDocument(ctx, dirs, dir, name, content, false)
}

func SaveDocument(ctx context.Context, dirs []Directory, scope Scope, name, content string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	return writeDocument(ctx, dirs, dir, name, content, true)
}

// SaveDocumentAs writes a skill directory to a new editable scope/name. Editable
// sources are moved after the new copy has been validated and written; read-only
// sources are copied so built-in Skills can be overridden without losing files.
func SaveDocumentAs(ctx context.Context, dirs []Directory, sourceScope Scope, sourceName string, targetScope Scope, targetName, content string) (Document, error) {
	sourceName = strings.TrimSpace(sourceName)
	targetName = strings.TrimSpace(targetName)
	if targetScope == "" {
		targetScope = sourceScope
	}
	if targetName == "" {
		targetName = sourceName
	}
	if sourceScope == targetScope && sourceName == targetName {
		return SaveDocument(ctx, dirs, sourceScope, sourceName, content)
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
	sourceSkillDir := filepath.Join(sourceDir.Path, sourceName)
	if _, err := os.Stat(filepath.Join(sourceSkillDir, SkillFileName)); err != nil {
		return Document{}, err
	}
	targetSkillDir := filepath.Join(targetDir.Path, targetName)
	if _, err := os.Stat(targetSkillDir); err == nil {
		return Document{}, fmt.Errorf("skill already exists in %s scope: %s", targetScope, targetName)
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
	return ReadDocument(ctx, dirs, targetScope, targetName)
}

func ListSkillFiles(ctx context.Context, dirs []Directory, scope Scope, name string) ([]SkillFile, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	_, dir, err := skillDirectory(dirs, scope, name)
	if err != nil {
		return nil, err
	}
	skillRoot, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return nil, err
	}
	defer skillRoot.Close()
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
		files = append(files, skillFileFromInfo(rel, info, dir.Writable))
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
	if ctx.Err() != nil {
		return FileDocument{}, ctx.Err()
	}
	skillDir, dir, err := skillDirectory(dirs, scope, name)
	if err != nil {
		return FileDocument{}, err
	}
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
	doc, err := ReadDocument(ctx, dirs, scope, name)
	if err != nil {
		return FileDocument{}, err
	}
	return FileDocument{
		Skill:   doc.SkillSummary,
		File:    skillFileFromInfo(rel, info, dir.Writable),
		Content: string(data),
	}, nil
}

func SaveSkillFile(ctx context.Context, dirs []Directory, scope Scope, name, filePath, content string) (FileDocument, error) {
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
	if err := atomicWriteSkillFile(skillRoot, rel, []byte(content), info.Mode().Perm()); err != nil {
		return FileDocument{}, err
	}
	info, err = skillRoot.Stat(filepath.FromSlash(rel))
	if err != nil {
		return FileDocument{}, err
	}
	doc, err := ReadDocument(ctx, dirs, scope, name)
	if err != nil {
		return FileDocument{}, err
	}
	return FileDocument{
		Skill:   doc.SkillSummary,
		File:    skillFileFromInfo(rel, info, dir.Writable),
		Content: content,
	}, nil
}

func DeleteDocument(ctx context.Context, dirs []Directory, scope Scope, name string) error {
	_ = ctx
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir.Path, name))
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

func writeDocument(ctx context.Context, dirs []Directory, dir Directory, name, content string, overwrite bool) (Document, error) {
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
	doc, err := ReadDocument(ctx, dirs, dir.Scope, name)
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

func atomicWriteSkillFile(root *os.Root, rel string, data []byte, mode os.FileMode) error {
	dir := path.Dir(filepath.ToSlash(rel))
	base := path.Base(filepath.ToSlash(rel))
	var random [8]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return err
	}
	tempRel := path.Join(dir, fmt.Sprintf(".%s.denova-%x.tmp", base, random[:]))
	tempPath := filepath.FromSlash(tempRel)
	targetPath := filepath.FromSlash(rel)
	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(tempPath, targetPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func skillFileFromInfo(rel string, info os.FileInfo, writable bool) SkillFile {
	return SkillFile{
		Path:      rel,
		Size:      info.Size(),
		Entry:     rel == SkillFileName,
		Editable:  writable && info.Size() <= maxSkillFileBytes,
		UpdatedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
	}
}
