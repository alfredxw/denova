package skills

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type record struct {
	skill     Skill
	summary   SkillSummary
	directory Directory
}

func SnapshotFor(ctx context.Context, dirs []Directory) (Snapshot, error) {
	dirs = dedupeDirectories(dirs)
	records := loadRecords(ctx, dirs)
	active := activeRecordKeys(records)
	summaries := make([]SkillSummary, 0, len(records))
	for _, rec := range records {
		item := rec.summary
		item.Active = active[recordKey(rec)]
		summaries = append(summaries, item)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Active != summaries[j].Active {
			return summaries[i].Active
		}
		if summaries[i].Name != summaries[j].Name {
			return summaries[i].Name < summaries[j].Name
		}
		return scopeRank(summaries[i].Scope) > scopeRank(summaries[j].Scope)
	})
	sharedEnabled := false
	for _, dir := range dirs {
		if dir.Scope == ScopeShared {
			sharedEnabled = !configureDirectory(ctx, dir).disabled
		}
	}
	return Snapshot{Scopes: scopeInfos(dirs), Skills: summaries, SharedEnabled: sharedEnabled}, nil
}

func (b *Backend) activeRecords(ctx context.Context) []record {
	dirs := make([]Directory, 0, len(b.dirs))
	for _, dir := range b.dirs {
		// A disabled shared library must not be parsed by an Agent at all.
		if !configureDirectory(ctx, dir).disabled {
			dirs = append(dirs, dir)
		}
	}
	records := loadRecords(ctx, dirs)
	active := make(map[string]record)
	for _, rec := range records {
		active[rec.skill.Name] = rec
	}
	out := make([]record, 0, len(active))
	for _, rec := range active {
		if !rec.summary.Enabled {
			continue
		}
		if !skillAllowedForAgent(rec, b.agentKind, b.overrides, b.explicitOnly) {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].skill.Name < out[j].skill.Name
	})
	return out
}

func skillAllowedForAgent(rec record, agentKind string, overrides map[string]bool, explicitOnly bool) bool {
	if agentKind == "" {
		return true
	}
	if enabled, ok := overrides[rec.skill.Name]; ok {
		return enabled
	}
	if explicitOnly {
		return false
	}
	return agentMatches(rec.skill.Agent, agentKind)
}

func agentMatches(agentField, agentKind string) bool {
	agentField = strings.TrimSpace(agentField)
	if agentField == "" {
		return true
	}
	for _, part := range strings.FieldsFunc(agentField, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" || strings.EqualFold(part, "all") || part == agentKind {
			return true
		}
	}
	return false
}

func normalizeOverrideMap(overrides map[string]bool) map[string]bool {
	if len(overrides) == 0 {
		return nil
	}
	out := make(map[string]bool, len(overrides))
	for name, enabled := range overrides {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = enabled
		}
	}
	return out
}

func loadRecords(ctx context.Context, dirs []Directory) []record {
	var records []record
	for _, dir := range dedupeDirectories(dirs) {
		dir = configureDirectory(ctx, dir)
		entries, err := os.ReadDir(dir.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				slog.ErrorContext(ctx, fmt.Sprintf("[skills] scan skill directory failed scope=%s path=%s err=%v", dir.Scope, dir.Path, err))
			}
			continue
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return records
			}
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if !entry.IsDir() && !(dir.Scope == ScopeShared && entry.Type()&os.ModeSymlink != 0) {
				continue
			}
			path := filepath.Join(dir.Path, entry.Name(), SkillFileName)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					slog.ErrorContext(ctx, fmt.Sprintf("[skills] read skill failed scope=%s path=%s err=%v", dir.Scope, path, readErr))
				}
				continue
			}
			rec, parseErr := parseRecord(ctx, dir, path, string(data))
			if parseErr != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[skills] parse skill failed scope=%s path=%s err=%v", dir.Scope, path, parseErr))
				continue
			}
			decorateRecord(ctx, &rec)
			records = append(records, rec)
		}
	}
	return records
}

func parseRecord(ctx context.Context, dir Directory, path, data string) (record, error) {
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	frontmatter, body, err := parseFrontmatter(data)
	if err != nil {
		return record{}, err
	}
	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return record{}, err
	}
	fm.Name = strings.TrimSpace(fm.Name)
	fm.Description = strings.TrimSpace(fm.Description)
	fm.Category = normalizeCategory(fm.Category)
	fm.Capabilities = normalizeCapabilities(fm.Capabilities)
	if dir.Scope == ScopeShared {
		// In shared Skills, `agent` names another tool's execution target; it is
		// not Denova's per-Agent availability allowlist.
		fm.Agent = ""
	}
	if err := ValidateName(fm.Name); err != nil {
		return record{}, err
	}
	if fm.Description == "" {
		return record{}, fmt.Errorf("skill description is required")
	}
	info, _ := os.Stat(path)
	updatedAt := ""
	if info != nil {
		updatedAt = info.ModTime().UTC().Format(time.RFC3339)
	}
	baseDir := filepath.Dir(path)
	return record{
		skill: Skill{
			FrontMatter:   fm,
			Content:       strings.TrimSpace(body),
			BaseDirectory: baseDir,
		},
		directory: dir,
		summary: SkillSummary{
			Name:         fm.Name,
			Description:  fm.Description,
			Category:     fm.Category,
			Capabilities: append([]string(nil), fm.Capabilities...),
			Context:      string(fm.Context),
			Agent:        fm.Agent,
			Model:        fm.Model,
			Scope:        dir.Scope,
			Path:         path,
			Editable:     dir.Writable,
			Enabled:      !dir.disabled && !dir.disabledSkills[string(dir.Scope)+":"+fm.Name],
			UpdatedAt:    updatedAt,
		},
	}, nil
}

func activeRecordKeys(records []record) map[string]bool {
	activeByName := make(map[string]record)
	for _, rec := range records {
		if rec.directory.disabled {
			continue
		}
		activeByName[rec.skill.Name] = rec
	}
	keys := make(map[string]bool, len(activeByName))
	for _, rec := range activeByName {
		keys[recordKey(rec)] = true
	}
	return keys
}

func recordKey(rec record) string {
	return string(rec.summary.Scope) + "\x00" + rec.summary.Path
}
