package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"denova/internal/agents/skills"
	"denova/internal/book/lore"
	"denova/internal/platform"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

func (s *Service) ReadPlan(ctx context.Context, id string) (Plan, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Plan{}, err
	}
	snapshot, err := revisionfile.Read(ctx, filepath.Join(s.root, "resource-exchange", "plans", id+".json"))
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(snapshot.Content, &plan); err != nil {
		return Plan{}, err
	}
	if plan.ID != id {
		return Plan{}, fmt.Errorf("invalid plan identity")
	}
	return plan, nil
}

// Apply commits only server-persisted plan data. The application must hold the
// Project mutation lease for plan.Installation.ProjectID for this call.
func (s *Service) Apply(ctx context.Context, id string) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, err := s.ReadPlan(ctx, id)
	if err != nil {
		return Installation{}, err
	}
	// A lost HTTP response may be retried safely with the same plan.
	txnSnapshot, err := revisionfile.Read(ctx, filepath.Join(s.root, "resource-exchange", "transactions", id+".json"))
	if err != nil {
		return Installation{}, err
	}
	if txnSnapshot.Exists {
		var txn transaction
		if err := json.Unmarshal(txnSnapshot.Content, &txn); err != nil {
			return Installation{}, err
		}
		if txn.State == "committed" {
			return plan.Installation, nil
		}
		return Installation{}, fmt.Errorf("installation operation already settled or needs recovery")
	}
	if time.Now().After(plan.ExpiresAt) {
		return Installation{}, fmt.Errorf("installation plan expired")
	}
	var preview Preview
	var dir string
	var at int
	if plan.BackupID == "" {
		preview, dir, err = s.loadPreview(plan.PreviewID)
		if err != nil {
			return Installation{}, err
		}
		at = slices.IndexFunc(preview.Candidates, func(item PackagePreview) bool { return item.ID == plan.CandidateID })
		if at < 0 {
			return Installation{}, fmt.Errorf("package candidate missing")
		}
	}
	selections := []platform.InstallSelection{}
	leases := []skills.MutationTarget{}
	for _, item := range plan.Items {
		if item.Action == "keep" {
			continue
		}
		if item.Extension != nil && plan.BackupID == "" {
			index := slices.IndexFunc(preview.Candidates[at].Resources, func(resource PreviewResource) bool { return resource.ID == item.ResourceID })
			if index < 0 {
				return Installation{}, fmt.Errorf("extension resource missing")
			}
			files, err := readFiles(filepath.Join(dir, "files", filepath.FromSlash(preview.Candidates[at].Resources[index].Path)))
			if err != nil {
				return Installation{}, err
			}
			raw, err := archiveBytes(files)
			if err != nil {
				return Installation{}, err
			}
			candidate, err := s.platform.PreviewZIP(item.Extension.Kind, raw)
			if err != nil {
				return Installation{}, err
			}
			defer s.platform.DiscardCandidate(candidate.ID)
			if candidate.Digest != item.Extension.Digest {
				return Installation{}, fmt.Errorf("frozen extension content changed")
			}
			selections = append(selections, platform.InstallSelection{CandidateID: candidate.ID, Grants: item.Grants, Reference: item.Action == "reference"})
		}
		if item.Local.Kind == "skill" {
			base := s.root
			if item.Local.ProjectID != "" {
				_, layout, err := s.registry.Resolve(item.Local.ProjectID, true)
				if err != nil {
					return Installation{}, err
				}
				base = layout.ContentRoot
			}
			leases = append(leases, skills.MutationTarget{Directory: skills.Directory{Path: filepath.Join(base, "skills"), Scope: skills.Scope(item.Local.Scope), Writable: true}, Name: item.Local.ID})
		}
	}
	commit := func(writes []platform.InstallWrite) error {
		changes := slices.Clone(plan.Changes)
		for _, write := range writes {
			changes = append(changes, fileChange{Target: FileTarget{Path: write.Path}, Expected: write.Expected, After: write.Content})
		}
		return commitFiles(ctx, s.root, s.registry, id, changes)
	}
	mutate := func() error {
		return skills.WithMutationLeases(ctx, leases, func() error {
			if plan.BackupID != "" && plan.PlatformState != "" {
				refs := []platform.PackageRef{}
				for _, item := range plan.Items {
					if item.Extension != nil {
						refs = append(refs, platform.PackageRef{Kind: item.Extension.Kind, ID: item.Extension.Manifest.ID})
					}
				}
				return s.platform.WithRestoreBarrier(plan.PlatformState, refs, func() error { return commit(nil) })
			}
			if len(selections) > 0 {
				return s.platform.WithInstallBatch(ctx, plan.PlatformState, selections, commit)
			}
			return commit(nil)
		})
	}
	if slices.ContainsFunc(plan.Items, func(item PlanItem) bool { return item.Local.Kind == "lore.item" }) {
		_, layout, err := s.registry.Resolve(plan.Installation.ProjectID, true)
		if err != nil {
			return Installation{}, err
		}
		err = lore.NewStore(layout.ContentRoot).WithMutationLock(mutate)
		if err != nil {
			return Installation{}, err
		}
	} else if err := mutate(); err != nil {
		return Installation{}, err
	}
	return plan.Installation, nil
}

func (s *Service) installations(ctx context.Context) ([]Installation, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "resource-exchange", "installations"))
	if os.IsNotExist(err) {
		return []Installation{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Installation{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		item, _, err := s.loadInstallation(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b Installation) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return result, nil
}

func (s *Service) Installations(ctx context.Context) ([]Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.installations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].LocalState = "unchanged"
		for _, binding := range items[i].Bindings {
			if binding.Local.Kind == "skill" {
				state, err := s.skillLocalState(ctx, binding)
				if err != nil {
					items[i].LocalState = "unavailable"
				} else if state != "unchanged" {
					items[i].LocalState = state
				}
				continue
			}
			for name, digest := range binding.Baseline {
				snapshot, err := s.snapshot(ctx, FileTarget{ProjectID: binding.Local.ProjectID, Path: name})
				if err != nil {
					items[i].LocalState = "unavailable"
					break
				}
				if !snapshot.Exists {
					items[i].LocalState = "missing"
				} else if snapshot.Revision != digest {
					items[i].LocalState = "modified"
				}
			}
			if strings.HasPrefix(binding.Local.Kind, "extension.") {
				installed, err := s.platform.List(platform.Kind(strings.TrimPrefix(binding.Local.Kind, "extension.")))
				if err != nil {
					return nil, err
				}
				if found := slices.IndexFunc(installed, func(value platform.Installed) bool { return value.ID == binding.Local.ID && !value.Removed }); found < 0 {
					items[i].LocalState = "missing"
				} else if installed[found].CurrentRelease != binding.SourceDigest {
					items[i].LocalState = "modified"
				} else if binding.Ownership == "reference" && !installed[found].Enabled {
					items[i].LocalState = "unavailable"
				}
			}
		}
	}
	return items, nil
}

// Detach stops source tracking and releases ownership. Content stays in its
// original domain library, where users already manage its lifecycle.
func (s *Service) Detach(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, snapshot, err := s.loadInstallation(ctx, id)
	if err != nil {
		return err
	}
	item.Tracking = "detached"
	item.UpdateMode = "manual"
	item.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	target, _ := resolveTarget(s.root, s.registry, installationTarget(id))
	_, err = revisionfile.ReplaceIfRevision(ctx, target, snapshot.Revision, raw, revisionfile.Options{})
	return err
}

func (s *Service) CheckUpdate(ctx context.Context, id string) (Preview, error) {
	item, _, err := s.loadInstallation(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	if item.Source.Kind == "file" || item.Tracking != "tracked" {
		return Preview{}, fmt.Errorf("installation has no tracked remote source")
	}
	// Record an attempt before downloading, without changing the installed baseline.
	s.mu.Lock()
	current, snapshot, readErr := s.loadInstallation(ctx, id)
	if readErr == nil && (current.Tracking != "tracked" || current.Source != item.Source || current.UpdatedAt != item.UpdatedAt) {
		readErr = ErrSourceChanged
	}
	if readErr == nil {
		current.CheckedAt = time.Now().UTC()
		current.RemoteState = "check_failed"
		raw, marshalErr := json.Marshal(current)
		readErr = marshalErr
		if readErr == nil {
			file, _ := resolveTarget(s.root, s.registry, installationTarget(id))
			_, readErr = revisionfile.ReplaceIfRevision(ctx, file, snapshot.Revision, raw, revisionfile.Options{})
		}
	}
	s.mu.Unlock()
	if readErr != nil {
		return Preview{}, readErr
	}
	source := item.Source
	source.Commit = ""
	preview, err := s.Preview(ctx, source, nil)
	if err != nil {
		return Preview{}, err
	}
	state := "identity_changed"
	for _, candidate := range preview.Candidates {
		if candidate.Package.ID != item.Package.ID {
			continue
		}
		state = "unchanged"
		for _, binding := range item.Bindings {
			if !slices.ContainsFunc(candidate.Resources, func(resource PreviewResource) bool {
				return resource.ID == binding.ResourceID && resource.Digest == binding.SourceDigest
			}) {
				state = "update_available"
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, snapshot, err = s.loadInstallation(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	if current.Tracking != "tracked" || current.Source != item.Source || current.UpdatedAt != item.UpdatedAt {
		return Preview{}, ErrSourceChanged
	}
	current.CheckedAt = time.Now().UTC()
	current.RemoteState = state
	for _, candidate := range preview.Candidates {
		if candidate.Package.ID != current.Package.ID {
			continue
		}
		for i := range current.Bindings {
			current.Bindings[i].UpstreamRemoved = !slices.ContainsFunc(candidate.Resources, func(resource PreviewResource) bool { return resource.ID == current.Bindings[i].ResourceID })
		}
	}
	current.PendingPreviewID = ""
	if state == "update_available" {
		current.PendingPreviewID = preview.ID
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return Preview{}, err
	}
	file, _ := resolveTarget(s.root, s.registry, installationTarget(id))
	_, err = revisionfile.ReplaceIfRevision(ctx, file, snapshot.Revision, raw, revisionfile.Options{})
	return preview, err
}

func (s *Service) DiscardPreview(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.installations(context.Background())
	if err != nil {
		return err
	}
	if slices.ContainsFunc(items, func(item Installation) bool {
		return item.Tracking == "tracked" && item.UpdateMode == "auto_apply" && item.PendingPreviewID == id
	}) {
		return nil
	}
	dir, err := s.previewPath(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
