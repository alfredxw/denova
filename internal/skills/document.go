package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReadDocument(ctx context.Context, dirs []Directory, scope Scope, name string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	path := filepath.Join(dir.Path, name, SkillFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	rec, err := parseRecord(ctx, dir, path, string(data))
	if err != nil {
		return Document{}, err
	}
	active := activeRecordKeys(loadRecords(ctx, dirs))
	rec.summary.Active = active[recordKey(rec)]
	return Document{SkillSummary: rec.summary, Content: string(data)}, nil
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

// SaveDocumentAs writes a skill to a new editable scope/name and removes the old
// editable document after the new copy has been validated and written.
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
	sourceDir, err := writableDirectoryForScope(dirs, sourceScope)
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
	targetPath := filepath.Join(targetDir.Path, targetName, SkillFileName)
	if _, err := os.Stat(targetPath); err == nil {
		return Document{}, fmt.Errorf("skill already exists in %s scope: %s", targetScope, targetName)
	} else if !os.IsNotExist(err) {
		return Document{}, err
	}
	if _, err := writeDocument(ctx, dirs, targetDir, targetName, content, false); err != nil {
		return Document{}, err
	}
	if err := os.RemoveAll(sourceSkillDir); err != nil {
		return Document{}, err
	}
	return ReadDocument(ctx, dirs, targetScope, targetName)
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
	path := filepath.Join(skillDir, SkillFileName)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return Document{}, fmt.Errorf("skill already exists: %s", name)
		}
	}
	rec, err := parseRecord(ctx, dir, path, content)
	if err != nil {
		return Document{}, err
	}
	if rec.skill.Name != name {
		return Document{}, fmt.Errorf("frontmatter name %q must match skill directory %q", rec.skill.Name, name)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Document{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Document{}, err
	}
	doc, err := ReadDocument(ctx, dirs, dir.Scope, name)
	if err != nil {
		return Document{}, err
	}
	return doc, nil
}
