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
	AutoUpdate    *bool        `json:"auto_update,omitempty"`
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
	if change.AutoUpdate != nil {
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
	case change.AutoUpdate != nil:
		_, err = skills.SetAutoUpdate(ctx, dirs, change.Scope, change.Name, *change.AutoUpdate)
	}
	if err != nil {
		return skills.Snapshot{}, err
	}
	slog.InfoContext(ctx, "Skill library preference changed", "scope", change.Scope, "name", change.Name,
		"enabled", change.Enabled, "shared_enabled", change.SharedEnabled, "auto_update", change.AutoUpdate)
	return skills.SnapshotFor(ctx, dirs)
}

func (s *Service) RefreshSkillUpdates(ctx context.Context, target SkillTarget, scope skills.Scope, name string, action skills.UpdateAction) ([]skills.UpdateResult, error) {
	if action != skills.CheckUpdate && action != skills.ApplyUpdate {
		return nil, fmt.Errorf("invalid Skill update action")
	}
	dirs, err := s.skillDirectories(target)
	if err != nil {
		return nil, err
	}
	if name != "" {
		return []skills.UpdateResult{skills.RefreshRemote(ctx, dirs, scope, name, action)}, nil
	}
	snapshot, err := skills.SnapshotFor(ctx, dirs)
	if err != nil {
		return nil, err
	}
	results := []skills.UpdateResult{}
	for _, item := range snapshot.Skills {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		if item.Remote == nil || !item.Editable {
			continue
		}
		if action == skills.ApplyUpdate && !item.Remote.UpdateAvailable {
			continue
		}
		results = append(results, skills.RefreshRemote(ctx, dirs, item.Scope, item.Name, action))
	}
	return results, nil
}
