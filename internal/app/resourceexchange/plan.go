package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
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
	mode := request.UpdateMode
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "notify" && mode != "auto_apply" {
		return Plan{}, fmt.Errorf("invalid update mode")
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
		if old.Source.Kind != preview.Source.Kind || old.Source.URL != preview.Source.URL || old.Source.Ref != preview.Source.Ref || old.Source.Path != preview.Source.Path {
			return Plan{}, ErrSourceChanged
		}
		installation.ID, installation.CreatedAt, installation.ProjectID = old.ID, old.CreatedAt, old.ProjectID
		installation.CheckedAt, installation.RemoteState = old.CheckedAt, "unchanged"
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
		case "lore.item", "game.opening", "project.cover":
			if resource.Kind == "project.cover" {
				local.ID = "cover"
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
		if selectedLocals[local] {
			return Plan{}, fmt.Errorf("multiple resources resolve to the same local identity; rename duplicate Skills")
		}
		selectedLocals[local] = true
		if resource.Extension == nil && localOwners[local] != "" && localOwners[local] != installation.ID {
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
		if !strings.HasPrefix(resource.Kind, "extension.") && resource.Kind != "skill" && resource.Kind != "project.cover" && resource.Kind != "style.reference" {
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
		binding := Binding{ResourceID: resource.ID, Local: local, Ownership: ownership, SourceDigest: resource.Digest, Baseline: map[string]string{}}
		plan.Installation.Bindings = append(plan.Installation.Bindings, binding)
		plan.Items = append(plan.Items, PlanItem{ResourceID: resource.ID, Name: resource.Name, Local: local, Action: action, Extension: resource.Extension, Grants: request.Grants[resource.ID]})
	}
	for i, resource := range selected {
		binding := &plan.Installation.Bindings[i]
		location := filepath.Join(dir, "files", filepath.FromSlash(resource.Path))
		if resource.Extension != nil {
			continue
		}
		if resource.Kind == "skill" {
			if previous, ok := oldBindings[resource.ID]; ok && !request.ReplaceModified {
				state, err := s.skillLocalState(ctx, previous)
				if err != nil {
					return Plan{}, err
				}
				if state != "unchanged" {
					return Plan{}, ErrLocalModified
				}
			}
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
				target, err = s.stageProject(ctx, dir, &extra, resource, binding.Local, raw, staged, expected, importedAssets)
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
			if oldBinding, ok := oldBindings[resource.ID]; ok {
				if baseline, tracked := oldBinding.Baseline[target.Path]; (tracked && snapshot.Revision != baseline || !tracked && snapshot.Exists && binding.Local.Kind == "skill") && !request.ReplaceModified {
					return Plan{}, ErrLocalModified
				}
			} else if snapshot.Exists && binding.Local.Kind == "skill" {
				return Plan{}, ErrSkillExists
			} else if snapshot.Exists && binding.Local.Kind == "project.cover" {
				if !request.ReplaceModified {
					return Plan{}, ErrLocalModified
				}
				plan.Items[i].Action = "update"
			}
			if content := staged[target]; content != nil {
				binding.Baseline[target.Path] = revisionfile.Revision(content)
			}
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
	}
	if !slices.ContainsFunc(plan.Installation.Bindings, func(binding Binding) bool { return binding.Local.ProjectID != "" }) {
		plan.Installation.ProjectID = ""
	}
	// Shared collection baselines describe the final collection, after all members.
	for i := range plan.Installation.Bindings {
		binding := &plan.Installation.Bindings[i]
		for _, target := range targets[binding.ResourceID] {
			if staged[target] != nil {
				binding.Baseline[target.Path] = revisionfile.Revision(staged[target])
			}
		}
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
