package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path"
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

func (s *Service) snapshot(ctx context.Context, target FileTarget) (revisionfile.Snapshot, error) {
	file, err := resolveTarget(s.root, s.registry, target)
	if err != nil {
		return revisionfile.Snapshot{}, err
	}
	return revisionfile.Read(ctx, file)
}
func installationTarget(id string) FileTarget {
	return FileTarget{Path: "resource-exchange/installations/" + id + ".json"}
}
func (s *Service) loadInstallation(ctx context.Context, id string) (Installation, revisionfile.Snapshot, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Installation{}, revisionfile.Snapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, installationTarget(id))
	if err != nil {
		return Installation{}, snapshot, err
	}
	var value Installation
	err = json.Unmarshal(snapshot.Content, &value)
	return value, snapshot, err
}

func (s *Service) Plan(ctx context.Context, request PlanRequest) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	preview, dir, err := s.loadPreview(request.PreviewID)
	if err != nil {
		return Plan{}, err
	}
	index := slices.IndexFunc(preview.Candidates, func(item PackagePreview) bool { return item.ID == request.CandidateID })
	if index < 0 {
		return Plan{}, fmt.Errorf("package candidate not found")
	}
	candidate := preview.Candidates[index]
	selected, err := selectResources(candidate.Resources, request.Resources)
	if err != nil {
		return Plan{}, err
	}
	if request.SharedResources != "" && request.SharedResources != "reuse" && request.SharedResources != "copy" {
		return Plan{}, fmt.Errorf("invalid shared resource import choice")
	}
	mode := request.UpdateMode
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "notify" && mode != "auto_apply" {
		return Plan{}, fmt.Errorf("invalid update mode")
	}
	review := &updateReview{choices: request.Resolutions, overrideByDetail: request.OverrideByDetail}
	if err := review.validate(); err != nil {
		return Plan{}, err
	}
	now := time.Now().UTC()
	installation := Installation{ID: uuid.NewString(), Package: candidate.Package, Source: preview.Source, ProjectID: request.ProjectID, Tracking: "tracked", UpdateMode: mode, CreatedAt: now, UpdatedAt: now, Bindings: []Binding{}}
	old := Installation{}
	recordRevision := revisionfile.MissingRevision
	if request.InstallationID != "" {
		var snapshot revisionfile.Snapshot
		old, snapshot, err = s.loadInstallation(ctx, request.InstallationID)
		if err != nil {
			return Plan{}, err
		}
		recordRevision = snapshot.Revision
		if old.Package.ID != candidate.Package.ID || old.Tracking != "tracked" {
			return Plan{}, fmt.Errorf("update package identity differs")
		}
		if preview.Source.Kind == "file" && old.Source.Kind != "file" {
			// Manual uploads replace the reviewed bytes, not the subscription.
			// Their commit is unknown; future checks compare resource digests.
			installation.Source = old.Source
			installation.Source.Commit = ""
		} else if old.Source.Kind != preview.Source.Kind || old.Source.URL != preview.Source.URL || old.Source.Ref != preview.Source.Ref || old.Source.Path != preview.Source.Path {
			return Plan{}, ErrSourceChanged
		}
		installation.ID, installation.CreatedAt, installation.ProjectID = old.ID, old.CreatedAt, old.ProjectID
		installation.CheckedAt, installation.RemoteState = now, "unchanged"
		if request.UpdateMode == "" {
			mode, installation.UpdateMode = old.UpdateMode, old.UpdateMode
		}
	}
	all, err := s.installations(ctx)
	if err != nil {
		return Plan{}, err
	}
	planID := uuid.NewString()
	if request.automatic {
		planID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("resource-update:"+installation.ID+":"+preview.ID)).String()
		if cached, err := s.ReadPlan(ctx, planID); err == nil && time.Now().Before(cached.ExpiresAt) {
			return cached, nil
		}
	}
	plan := Plan{ID: planID, PreviewID: preview.ID, CandidateID: candidate.ID, ExpiresAt: now.Add(time.Hour), Installation: installation, Items: []PlanItem{}}
	if request.automatic {
		plan.ExpiresAt = preview.ExpiresAt
	}
	staged := map[FileTarget][]byte{}
	importedAssets := map[FileTarget]lore.Asset{}
	expected := map[FileTarget]string{}
	targets := map[string][]FileTarget{}
	refs := map[string]string{}
	localOwners := map[LocalRef]string{}
	for _, item := range all {
		if item.Tracking == "tracked" {
			for _, binding := range item.Bindings {
				if binding.Ownership == "owned" {
					localOwners[binding.Local] = item.ID
				}
			}
		}
	}
	selectedLocals := map[LocalRef]bool{}
	oldBindings := map[string]Binding{}
	for _, binding := range old.Bindings {
		oldBindings[binding.ResourceID] = binding
	}
	// Assign every identity before rewriting typed references, independent of order.
	for _, resource := range selected {
		var referenceBaseline map[string]string
		local := LocalRef{Kind: resource.Kind, Scope: "global", ID: uuid.NewString()}
		ownership := "owned"
		action := "create"
		switch resource.Kind {
		case "skill":
			local.Scope = request.SkillScope
			if local.Scope == "" {
				local.Scope = "user"
			}
			if local.Scope != "user" && local.Scope != "workspace" {
				return Plan{}, fmt.Errorf("invalid Skill scope")
			}
			local.ID = resource.Name
			if request.Names[resource.ID] != "" {
				local.ID = request.Names[resource.ID]
			}
			if err := skills.ValidateName(local.ID); err != nil {
				return Plan{}, err
			}
			if local.Scope == "workspace" {
				local.ProjectID = installation.ProjectID
			}
		case "style.reference":
			local.ID += ".md"
		case "lore.collection", "game.openings", "project.cover", "project.creator":
			switch resource.Kind {
			case "project.cover":
				local.ID = "cover"
			case "project.creator":
				local.ID = "creator"
			}
			local.Scope = "project"
			local.ProjectID = installation.ProjectID
		case "extension.plugin", "extension.game":
			local.ID = resource.Extension.Manifest.ID
		}
		if binding, ok := oldBindings[resource.ID]; ok {
			if binding.Local.Kind != resource.Kind {
				return Plan{}, fmt.Errorf("resource kind changed")
			}
			local, ownership, action = binding.Local, binding.Ownership, "update"
		}
		if (local.Scope == "project" || local.Scope == "workspace") && local.ProjectID == "" {
			return Plan{}, fmt.Errorf("select a Project for Project resources")
		}
		if local.ProjectID != "" {
			if _, _, err := s.registry.Resolve(local.ProjectID, true); err != nil {
				return Plan{}, err
			}
		}
		if old.ID == "" {
			var reused bool
			local, referenceBaseline, reused, err = s.sharedImport(ctx, local, resource, installation, all, selectedLocals, request.SharedResources)
			if err != nil {
				return Plan{}, err
			}
			if reused {
				ownership, action = "reference", "reference"
				slog.InfoContext(ctx, "resource_import_reused", "installation", installation.ID, "resource", resource.ID, "kind", local.Kind, "local_id", local.ID)
			}
		} else if ownership == "reference" && resource.Extension == nil {
			action = "reference"
			referenceBaseline = maps.Clone(oldBindings[resource.ID].Baseline)
		}
		if selectedLocals[local] && ownership != "reference" {
			return Plan{}, fmt.Errorf("multiple resources resolve to the same local identity; rename duplicate Skills")
		}
		selectedLocals[local] = true
		if resource.Extension == nil && ownership != "reference" && localOwners[local] != "" && localOwners[local] != installation.ID {
			return Plan{}, ErrResourceOwned
		}
		if resource.Extension != nil {
			items, err := s.platform.List(resource.Extension.Kind)
			if err != nil {
				return Plan{}, err
			}
			at := slices.IndexFunc(items, func(item platform.Installed) bool { return item.ID == local.ID && !item.Removed })
			owner := ""
			for _, existing := range all {
				if existing.Tracking != "tracked" {
					continue
				}
				for _, binding := range existing.Bindings {
					if binding.Local == local && binding.Ownership == "owned" {
						owner = existing.ID
					}
				}
			}
			if ownership == "reference" {
				if at < 0 || items[at].CurrentRelease != resource.Extension.Digest || !items[at].Enabled {
					return Plan{}, ErrReferenceChanged
				}
				action = "reference"
			} else if at >= 0 && old.ID == "" {
				if owner == "" {
					action = "update"
				} else {
					if items[at].CurrentRelease != resource.Extension.Digest || !items[at].Enabled {
						return Plan{}, fmt.Errorf("extension identity already exists with different content or state: %s", local.ID)
					}
					ownership, action = "reference", "reference"
				}
			} else if owner != "" && owner != installation.ID {
				return Plan{}, fmt.Errorf("extension is owned by another installation")
			}
		}
		refs[resource.Kind+":"+resource.ID] = local.ID
		refs[resource.Kind+":"+resource.Path] = local.ID
		if !strings.HasPrefix(resource.Kind, "extension.") && resource.Kind != "skill" && resource.Kind != "project.cover" && resource.Kind != "project.creator" && resource.Kind != "style.reference" {
			raw, err := os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(resource.Path)))
			if err != nil {
				return Plan{}, err
			}
			var identity struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &identity); err != nil {
				return Plan{}, err
			}
			if identity.ID != "" {
				key := resource.Kind + ":" + identity.ID
				if previous, ok := refs[key]; ok && previous != local.ID {
					return Plan{}, fmt.Errorf("ambiguous typed resource identity")
				}
				refs[key] = local.ID
			}
		}
		binding := Binding{Requires: slices.Clone(resource.Requires), ResourceID: resource.ID, Local: local, Ownership: ownership, SourceDigest: resource.Digest, Baseline: map[string]string{}}
		if ownership == "reference" && resource.Extension == nil {
			binding.Baseline = referenceBaseline
		}
		binding.Members = maps.Clone(oldBindings[resource.ID].Members)
		if collectionPath(resource.Kind) != "" {
			binding.Baseline = maps.Clone(oldBindings[resource.ID].Baseline)
			if binding.Baseline == nil {
				binding.Baseline = map[string]string{}
			}
		}
		if resource.Extension != nil {
			binding.AppliedRelease = resource.Extension.Digest
		}
		plan.Installation.Bindings = append(plan.Installation.Bindings, binding)
		plan.Items = append(plan.Items, PlanItem{ResourceID: resource.ID, Name: resource.Name, Local: local, Action: action, Extension: resource.Extension, Grants: request.Grants[resource.ID]})
	}
	blockedNew := map[string]bool{}
	for i, resource := range selected {
		binding := &plan.Installation.Bindings[i]
		if binding.Ownership == "reference" && resource.Extension == nil {
			review.items = append(review.items, UpdateItem{ResourceID: resource.ID, Name: resource.Name, State: "unchanged"})
			continue
		}
		if oldBindings[resource.ID].SourceDigest != resource.Digest && slices.ContainsFunc(resource.Requires, review.pending) {
			review.items = append(review.items, UpdateItem{ResourceID: resource.ID, Name: resource.Name, State: "blocked"})
			plan.Items[i].Action = "keep"
			if previous, ok := oldBindings[resource.ID]; ok {
				*binding = previous
			} else {
				blockedNew[resource.ID] = true
			}
			continue
		}
		location := filepath.Join(dir, "files", filepath.FromSlash(resource.Path))
		if resource.Extension != nil {
			if previous, ok := oldBindings[resource.ID]; ok && previous.Ownership == "owned" {
				if err := s.reviewExtension(previous, binding, &plan.Items[i], review); err != nil {
					return Plan{}, err
				}
			} else {
				state := plan.Items[i].Action
				if state == "reference" {
					state = "unchanged"
				}
				review.items = append(review.items, UpdateItem{ResourceID: resource.ID, Name: resource.Name, State: state})
			}
			continue
		}
		if resource.Kind == "skill" {
			files, err := readFiles(location)
			if err != nil {
				return Plan{}, err
			}
			delete(files, ".denova-source.json")
			if binding.Local.ID != resource.Name {
				renamed, err := skills.RenameDocument(string(files["SKILL.md"]), binding.Local.ID)
				if err != nil {
					return Plan{}, err
				}
				files["SKILL.md"] = []byte(renamed)
			}
			if _, ok := files["SKILL.md"]; !ok {
				return Plan{}, fmt.Errorf("Skill resource must point at its SKILL.md directory")
			}
			prefix := path.Join("skills", binding.Local.ID)
			for name, content := range files {
				target := FileTarget{ProjectID: binding.Local.ProjectID, Path: path.Join(prefix, name)}
				staged[target] = content
				targets[resource.ID] = append(targets[resource.ID], target)
			}
			if oldBinding, ok := oldBindings[resource.ID]; ok {
				for name := range oldBinding.Baseline {
					target := FileTarget{ProjectID: binding.Local.ProjectID, Path: name}
					if _, ok := staged[target]; !ok {
						targets[resource.ID] = append(targets[resource.ID], target)
						staged[target] = nil
					}
				}
			}
		} else {
			raw, err := os.ReadFile(location)
			if err != nil {
				return Plan{}, err
			}
			var target FileTarget
			if binding.Local.Scope == "project" {
				var extra []FileTarget
				target, err = s.stageProject(ctx, dir, &extra, resource, binding, raw, review, staged, expected, importedAssets)
				targets[resource.ID] = append(targets[resource.ID], extra...)
			} else {
				var content []byte
				target.Path, content, err = stageDefinition(resource, binding.Local.ID, raw, refs)
				staged[target] = content
			}
			if err != nil {
				return Plan{}, err
			}
			targets[resource.ID] = append(targets[resource.ID], target)
		}
		for _, target := range targets[resource.ID] {
			snapshot, err := s.snapshot(ctx, target)
			if err != nil {
				return Plan{}, err
			}
			if _, ok := expected[target]; !ok {
				expected[target] = snapshot.Revision
			}
			if binding.Local.Kind == "project.creator" && snapshot.Exists && old.ID == "" {
				if !request.ReplaceModified {
					return Plan{}, ErrLocalModified
				}
				plan.Items[i].Action = "update"
			}
			if _, updating := oldBindings[resource.ID]; !updating && snapshot.Exists && binding.Local.Kind == "skill" {
				return Plan{}, ErrSkillExists
			} else if snapshot.Exists && binding.Local.Kind == "project.cover" && old.ID == "" {
				if !request.ReplaceModified {
					return Plan{}, ErrLocalModified
				}
				plan.Items[i].Action = "update"
			}
			if content := staged[target]; content != nil && target.Path != collectionPath(binding.Local.Kind) {
				binding.Baseline[target.Path] = fileContentDigest(binding.Local.Kind, content)
			}
		}
		if previous, ok := oldBindings[resource.ID]; ok {
			if collectionPath(resource.Kind) == "" {
				if err := s.reviewFiles(ctx, previous, binding, &plan.Items[i], targets[resource.ID], staged, expected, review, &plan); err != nil {
					return Plan{}, err
				}
			} else if review.pending(resource.ID) {
				binding.SourceDigest = previous.SourceDigest
			}
		} else if collectionPath(resource.Kind) == "" {
			review.items = append(review.items, UpdateItem{ResourceID: resource.ID, Name: resource.Name, State: "create"})
		}
	}
	plan.Installation.Bindings = slices.DeleteFunc(plan.Installation.Bindings, func(binding Binding) bool { return blockedNew[binding.ResourceID] })
	plan.Updates = review.items
	plan.ImportOverrides = review.overrides
	plan.ImportSkipped = review.skipped
	plan.Installation.ReviewedSource = reviewedSource(candidate)
	// Automatic grants cannot acknowledge additions or dependency changes that
	// require the user to review a broader resource selection.
	if request.automatic && len(selected) < len(candidate.Resources) {
		plan.Installation.ReviewedSource = old.ReviewedSource
		plan.Installation.RemoteState = "update_available"
	}
	for _, item := range review.items {
		if item.State == "conflict" || item.State == "blocked" {
			plan.Installation.RemoteState = "update_available"
			plan.Installation.ReviewedSource = old.ReviewedSource
			break
		}
	}
	// Only resources selected in this plan can be adopted during import.
	if len(request.GameDefaultsFields) > 0 {
		available, err := resolvePackageGameDefaults(candidate, plan.Installation.Bindings, importedAssets)
		if err != nil {
			return Plan{}, err
		}
		if available != nil && candidate.GameDefaults != nil && candidate.GameDefaults.DefaultBackground != nil && old.GameDefaults != nil && available.DefaultBackground == nil {
			bg := candidate.GameDefaults.DefaultBackground
			if slices.ContainsFunc(selected, func(resource PreviewResource) bool { return resource.ID == bg.ResourceID }) && !review.pending(bg.ResourceID) {
				available.DefaultBackground = old.GameDefaults.DefaultBackground
			}
		}
		fields := request.GameDefaultsFields
		if available != nil && available.DefaultBackground == nil && slices.Contains(fields, "default_background") && old.ID != "" && candidate.GameDefaults.DefaultBackground != nil {
			bg := candidate.GameDefaults.DefaultBackground
			if slices.ContainsFunc(selected, func(resource PreviewResource) bool { return resource.ID == bg.ResourceID }) {
				// A retained or conflicted Lore item has no incoming asset mapping.
				// Keep its review accessible without silently dropping adoption.
				plan.GameDefaultsUnavailable = []string{"default_background"}
				fields = slices.DeleteFunc(slices.Clone(fields), func(field string) bool { return field == "default_background" })
			}
		}
		if _, err := available.Select(fields); err != nil {
			return Plan{}, err
		}
	}
	// Omitted and upstream-removed members stay local and retain their baselines.
	// A package update never doubles as resource deletion or source reassignment.
	for _, binding := range old.Bindings {
		if slices.ContainsFunc(plan.Installation.Bindings, func(next Binding) bool { return next.ResourceID == binding.ResourceID }) {
			continue
		}
		binding.UpstreamRemoved = !slices.ContainsFunc(candidate.Resources, func(resource PreviewResource) bool { return resource.ID == binding.ResourceID })
		plan.Installation.Bindings = append(plan.Installation.Bindings, binding)
		plan.Items = append(plan.Items, PlanItem{ResourceID: binding.ResourceID, Name: binding.Local.ID, Local: binding.Local, Action: "keep"})
		state := "keep"
		if binding.UpstreamRemoved {
			state = "upstream_removed"
		}
		plan.Updates = append(plan.Updates, UpdateItem{ResourceID: binding.ResourceID, Name: binding.Local.ID, State: state})
	}
	if !slices.ContainsFunc(plan.Installation.Bindings, func(binding Binding) bool { return binding.Local.ProjectID != "" }) {
		plan.Installation.ProjectID = ""
	}
	plan.Installation.GameDefaults, err = resolvePackageGameDefaults(candidate, plan.Installation.Bindings, importedAssets)
	if err != nil {
		return Plan{}, err
	}
	// Retaining an installed Lore collection also retains its available background
	// recommendation. A partial update has no new asset mapping for that collection.
	if candidate.GameDefaults != nil && candidate.GameDefaults.DefaultBackground != nil && old.GameDefaults != nil && old.GameDefaults.DefaultBackground != nil && plan.Installation.GameDefaults.DefaultBackground == nil {
		bg := candidate.GameDefaults.DefaultBackground
		for _, binding := range old.Bindings {
			if binding.ResourceID == bg.ResourceID && binding.Members[bg.ItemID].ID == old.GameDefaults.DefaultBackground.ItemID {
				plan.Installation.GameDefaults.DefaultBackground = old.GameDefaults.DefaultBackground
			}
		}
	}
	if err := s.stageGameDefaults(ctx, request, &plan, staged, expected); err != nil {
		return Plan{}, err
	}
	if slices.ContainsFunc(plan.Items, func(item PlanItem) bool { return item.Extension != nil }) {
		plan.PlatformState, err = s.platform.InstallState()
		if err != nil {
			return Plan{}, err
		}
	}
	if err := validateUpdateMode(mode, plan.Installation); err != nil {
		return Plan{}, err
	}
	if old.ID != "" {
		conflicts := 0
		for _, item := range plan.Updates {
			if item.State == "conflict" || item.State == "blocked" {
				conflicts++
			}
		}
		slog.InfoContext(ctx, "resource_update_planned", "installation", old.ID, "units", len(plan.Updates), "pending", conflicts, "unavailable_defaults", plan.GameDefaultsUnavailable)
	}
	record, err := json.Marshal(plan.Installation)
	if err != nil {
		return Plan{}, err
	}
	recordTarget := installationTarget(installation.ID)
	staged[recordTarget], expected[recordTarget] = record, recordRevision
	keys := make([]FileTarget, 0, len(staged))
	for target := range staged {
		keys = append(keys, target)
	}
	slices.SortFunc(keys, func(a, b FileTarget) int { return strings.Compare(a.ProjectID+"/"+a.Path, b.ProjectID+"/"+b.Path) })
	for _, target := range keys {
		plan.Changes = append(plan.Changes, fileChange{Target: target, Expected: expected[target], After: staged[target], Delete: staged[target] == nil})
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return Plan{}, err
	}
	_, err = revisionfile.ReplaceIfRevision(ctx, filepath.Join(s.root, "resource-exchange", "plans", plan.ID+".json"), revisionfile.MissingRevision, raw, revisionfile.Options{})
	return plan, err
}
