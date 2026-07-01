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
	for _, rec := range b.activeRecords(ctx) {
		if rec.skill.Name == name {
			return rec.skill, nil
		}
	}
	return einoskill.Skill{}, fmt.Errorf("skill not found: %s", name)
}
