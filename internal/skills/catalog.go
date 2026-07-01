package skills

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
	"gopkg.in/yaml.v3"
)

type record struct {
	skill   einoskill.Skill
	summary SkillSummary
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
	return Snapshot{Scopes: scopeInfos(dirs), Skills: summaries}, nil
}

func (b *Backend) activeRecords(ctx context.Context) []record {
	records := loadRecords(ctx, b.dirs)
	active := make(map[string]record)
	for _, rec := range records {
		active[rec.skill.Name] = rec
	}
	out := make([]record, 0, len(active))
	for _, rec := range active {
		if !skillAllowedForAgent(rec, b.agentKind, b.overrides) {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].skill.Name < out[j].skill.Name
	})
	return out
}

func skillAllowedForAgent(rec record, agentKind string, overrides map[string]bool) bool {
	if agentKind == "" {
		return true
	}
	if enabled, ok := overrides[rec.skill.Name]; ok {
		return enabled
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
		entries, err := os.ReadDir(dir.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("[skills] scan skill directory failed scope=%s path=%s err=%v", dir.Scope, dir.Path, err)
			}
			continue
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return records
			}
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir.Path, entry.Name(), SkillFileName)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					log.Printf("[skills] read skill failed scope=%s path=%s err=%v", dir.Scope, path, readErr)
				}
				continue
			}
			rec, parseErr := parseRecord(ctx, dir, path, string(data))
			if parseErr != nil {
				log.Printf("[skills] parse skill failed scope=%s path=%s err=%v", dir.Scope, path, parseErr)
				continue
			}
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
	var fm einoskill.FrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return record{}, err
	}
	fm.Name = strings.TrimSpace(fm.Name)
	fm.Description = strings.TrimSpace(fm.Description)
	if err := ValidateName(fm.Name); err != nil {
		return record{}, err
	}
	if fm.Description == "" {
		return record{}, fmt.Errorf("skill description is required")
	}
	info, _ := os.Stat(path)
	updatedAt := ""
	if info != nil {
		updatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	baseDir := filepath.Dir(path)
	return record{
		skill: einoskill.Skill{
			FrontMatter:   fm,
			Content:       strings.TrimSpace(body),
			BaseDirectory: baseDir,
		},
		summary: SkillSummary{
			Name:        fm.Name,
			Description: fm.Description,
			Context:     string(fm.Context),
			Agent:       fm.Agent,
			Model:       fm.Model,
			Scope:       dir.Scope,
			Path:        path,
			Editable:    dir.Writable,
			UpdatedAt:   updatedAt,
		},
	}, nil
}

func activeRecordKeys(records []record) map[string]bool {
	activeByName := make(map[string]record)
	for _, rec := range records {
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
