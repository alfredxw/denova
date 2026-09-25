package resourcecatalog

import (
	"context"
	"fmt"
	"log/slog"

	"denova/internal/agents/skills"
)

// SkillPreferenceChange changes exactly one library preference. Shared and
// builtin/user preferences are global; workspace availability is Project-local.
type SkillPreferenceChange struct {
	Scope         skills.Scope `json:"scope"`
	Name          string       `json:"name"`
	Enabled       *bool        `json:"enabled,omitempty"`
	SharedEnabled *bool        `json:"shared_enabled,omitempty"`
}

func (s *Service) SetSkillPreference(ctx context.Context, target SkillTarget, change SkillPreferenceChange) (skills.Snapshot, error) {
	dirs, err := s.skillDirectories(target)
	if err != nil {
		return skills.Snapshot{}, err
	}
	count := 0
	if change.SharedEnabled != nil {
		count++
	}
	if change.Enabled != nil {
		count++
	}
	if count != 1 {
		return skills.Snapshot{}, fmt.Errorf("exactly one Skill preference is required")
	}
	switch {
	case change.SharedEnabled != nil:
		err = skills.SetSharedEnabled(ctx, dirs, *change.SharedEnabled)
	case change.Enabled != nil:
		err = skills.SetLibraryEnabled(ctx, dirs, change.Scope, change.Name, *change.Enabled)
	}
	if err != nil {
		return skills.Snapshot{}, err
	}
	slog.InfoContext(ctx, "Skill library preference changed", "scope", change.Scope, "name", change.Name,
		"enabled", change.Enabled, "shared_enabled", change.SharedEnabled)
	return skills.SnapshotFor(ctx, dirs)
}
