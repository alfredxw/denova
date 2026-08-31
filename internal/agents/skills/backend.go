package skills

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Backend resolves the effective Skill catalog across all configured scopes.
type Backend struct {
	dirs         []Directory
	agentKind    string
	overrides    map[string]bool
	explicitOnly bool
}

func NewBackend(dirs []Directory) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs)}
}

func NewAgentBackend(dirs []Directory, agentKind string, overrides map[string]bool) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs), agentKind: strings.TrimSpace(agentKind), overrides: normalizeOverrideMap(overrides)}
}

// NewAgentBackendWithPolicy supports an explicit-only catalog without
// expanding settings into one boolean per installed Skill.
func NewAgentBackendWithPolicy(dirs []Directory, agentKind string, overrides map[string]bool, explicitOnly bool) *Backend {
	return &Backend{
		dirs: dedupeDirectories(dirs), agentKind: strings.TrimSpace(agentKind),
		overrides: normalizeOverrideMap(overrides), explicitOnly: explicitOnly,
	}
}

func (b *Backend) List(ctx context.Context) ([]FrontMatter, error) {
	records := b.activeRecords(ctx)
	matters := make([]FrontMatter, 0, len(records))
	for _, rec := range records {
		matters = append(matters, rec.skill.FrontMatter)
	}
	sort.Slice(matters, func(i, j int) bool {
		return matters[i].Name < matters[j].Name
	})
	return matters, nil
}

func (b *Backend) Get(ctx context.Context, name string) (Skill, error) {
	for _, rec := range b.activeRecords(ctx) {
		if rec.skill.Name == name {
			return rec.skill, nil
		}
	}
	return Skill{}, fmt.Errorf("skill not found: %s", name)
}
