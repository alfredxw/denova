package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"denova/internal/revisionfile"
)

func automaticKinds(bindings []Binding) bool {
	for _, binding := range bindings {
		kind := binding.Local.Kind
		if kind != "skill" && kind != "style.reference" && !strings.HasPrefix(kind, "preset.") {
			return false
		}
	}
	return len(bindings) > 0
}
func validateUpdateMode(mode string, item Installation) error {
	if mode != "manual" && mode != "notify" && mode != "auto_apply" {
		return fmt.Errorf("unknown update mode")
	}
	if mode != "manual" && (item.Source.Kind == "file" || item.Tracking != "tracked") {
		return fmt.Errorf("update policy requires a tracked remote source")
	}
	if mode == "auto_apply" && !automaticKinds(item.Bindings) {
		return fmt.Errorf("automatic update is unavailable for this composition")
	}
	return nil
}
func (s *Service) SetUpdateMode(ctx context.Context, id, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, snapshot, err := s.loadInstallation(ctx, id)
	if err != nil {
		return err
	}
	if err := validateUpdateMode(mode, item); err != nil {
		return err
	}
	item.UpdateMode = mode
	item.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	file, _ := resolveTarget(s.root, s.registry, installationTarget(id))
	_, err = revisionfile.ReplaceIfRevision(ctx, file, snapshot.Revision, raw, revisionfile.Options{})
	return err
}

// UpdateDue visits only opted-in installations. Each attempt, including failure,
// is persisted before network work so a disconnected source is not hot-looped.
// Application submission retains its normal Project gates and cancellation.
func (s *Service) UpdateDue(ctx context.Context, now time.Time, apply func(context.Context, string) (Installation, error)) {
	items, err := s.Installations(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "resource_update_scan_failed", "error", err)
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		if item.Tracking != "tracked" || item.UpdateMode == "manual" || item.Source.Kind == "file" {
			continue
		}
		var preview Preview
		if item.UpdateMode == "auto_apply" && item.PendingPreviewID != "" {
			preview, _, _ = s.loadPreview(item.PendingPreviewID)
		}
		if preview.ID == "" {
			if !item.CheckedAt.IsZero() && now.Sub(item.CheckedAt) < 24*time.Hour {
				continue
			}
			preview, err = s.CheckUpdate(ctx, item.ID)
			if err != nil {
				slog.WarnContext(ctx, "resource_update_check_failed", "installation", item.ID, "error", err)
				continue
			}
		}
		func() {
			defer s.DiscardPreview(preview.ID)
			if item.LocalState != "unchanged" {
				return
			}
			current, _, err := s.loadInstallation(ctx, item.ID)
			if err != nil || current.RemoteState != "update_available" || current.UpdateMode != "auto_apply" || !automaticKinds(current.Bindings) {
				return
			}
			at := slices.IndexFunc(preview.Candidates, func(candidate PackagePreview) bool { return candidate.Package.ID == current.Package.ID })
			if at < 0 {
				return
			}
			candidate := preview.Candidates[at]
			// Never add resources or remove bindings under an earlier automatic grant.
			selected := []string{}
			for _, binding := range current.Bindings {
				if !slices.ContainsFunc(candidate.Resources, func(resource PreviewResource) bool {
					return resource.ID == binding.ResourceID && resource.Kind == binding.Local.Kind
				}) {
					return
				}
				selected = append(selected, binding.ResourceID)
			}
			closure, err := selectResources(candidate.Resources, selected)
			if err != nil || len(closure) != len(selected) {
				return
			}
			plan, err := s.Plan(ctx, PlanRequest{automatic: true, PreviewID: preview.ID, CandidateID: candidate.ID, Resources: selected, InstallationID: current.ID, UpdateMode: "auto_apply", ProjectID: current.ProjectID})
			if err != nil {
				slog.InfoContext(ctx, "resource_auto_update_deferred", "installation", item.ID, "error", err)
				return
			}
			if _, err := apply(ctx, plan.ID); err != nil {
				slog.WarnContext(ctx, "resource_auto_update_failed", "installation", item.ID, "error", err)
			}
		}()
	}
}
