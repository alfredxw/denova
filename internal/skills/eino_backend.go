package skills

import (
	"context"
	"fmt"
	"sort"
	"strings"

	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
)

// Backend adapts multiple Nova skill directories to Eino's skill.Backend.
type Backend struct {
	dirs      []Directory
	agentKind string
	overrides map[string]bool
}

func NewBackend(dirs []Directory) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs)}
}

func NewAgentBackend(dirs []Directory, agentKind string, overrides map[string]bool) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs), agentKind: strings.TrimSpace(agentKind), overrides: normalizeOverrideMap(overrides)}
}

func (b *Backend) List(ctx context.Context) ([]einoskill.FrontMatter, error) {
	records := b.activeRecords(ctx)
	matters := make([]einoskill.FrontMatter, 0, len(records))
	for _, rec := range records {
		matters = append(matters, rec.skill.FrontMatter)
	}
	sort.Slice(matters, func(i, j int) bool {
		return matters[i].Name < matters[j].Name
	})
	return matters, nil
}

func (b *Backend) Get(ctx context.Context, name string) (einoskill.Skill, error) {
	records := b.activeRecords(ctx)
	for _, rec := range records {
		if rec.skill.Name == name {
			return b.resolveDepends(ctx, records, rec, map[string]bool{name: true}), nil
		}
	}
	return einoskill.Skill{}, fmt.Errorf("skill not found: %s", name)
}

// resolveDepends prepends dependency skill content before the requesting skill's
// own content so the model receives shared conventions without an extra tool call.
func (b *Backend) resolveDepends(ctx context.Context, records []record, rec record, visited map[string]bool) einoskill.Skill {
	if len(rec.depends) == 0 {
		return rec.skill
	}
	var parts []string
	for _, dep := range rec.depends {
		if visited[dep] {
			continue
		}
		visited[dep] = true
		for _, depRec := range records {
			if depRec.skill.Name == dep {
				resolved := b.resolveDepends(ctx, records, depRec, visited)
				parts = append(parts, resolved.Content)
				break
			}
		}
	}
	if len(parts) == 0 {
		return rec.skill
	}
	parts = append(parts, rec.skill.Content)
	out := rec.skill
	out.Content = strings.Join(parts, "\n\n---\n\n")
	return out
}
