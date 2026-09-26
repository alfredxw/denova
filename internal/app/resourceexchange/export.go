package resourceexchange

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"denova/internal/agents/skills"
	"denova/internal/app/resourcecatalog"
	"denova/internal/book/lore"
	"denova/internal/platform"
	"denova/internal/style"
	"github.com/google/uuid"
)

type ExportResource struct {
	Local       LocalRef `json:"local"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
}
type ExportRequest struct {
	InstallationID string      `json:"installation_id,omitempty"`
	Package        PackageInfo `json:"package"`
	Resources      []LocalRef  `json:"resources"`
	Native         bool        `json:"native,omitempty"`
}

func summaries[T any](kind string, items []T) ([]ExportResource, error) {
	result := []ExportResource{}
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		var identity struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Description       string `json:"description"`
			Invalid           bool   `json:"invalid"`
			Custom            bool   `json:"custom"`
			BuiltinOverridden bool   `json:"builtin_overridden"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return nil, err
		}
		// The picker only offers user content; shipped defaults remain available
		// to the exporter when required as dependencies of a selected resource.
		if !identity.Invalid && (identity.Custom || identity.BuiltinOverridden) {
			result = append(result, ExportResource{Local: LocalRef{Kind: kind, Scope: "global", ID: identity.ID}, Name: identity.Name, Description: identity.Description})
		}
	}
	return result, nil
}

func (s *Service) ExportResources(ctx context.Context, projectID string) ([]ExportResource, error) {
	result := []ExportResource{}
	// Keep typed catalog APIs as the authority for builtin and custom definitions.
	a, err := s.catalog.Tellers()
	if err != nil {
		return nil, err
	}
	a1, err := summaries("preset.narrative", a)
	if err != nil {
		return nil, err
	}
	result = append(result, a1...)
	b, err := s.catalog.ImagePresets()
	if err != nil {
		return nil, err
	}
	b1, err := summaries("preset.image", b)
	if err != nil {
		return nil, err
	}
	result = append(result, b1...)
	c, err := s.catalog.GamePlanningTemplates()
	if err != nil {
		return nil, err
	}
	c1, err := summaries("preset.game_planning", c)
	if err != nil {
		return nil, err
	}
	result = append(result, c1...)
	d, err := s.catalog.EventPackages()
	if err != nil {
		return nil, err
	}
	d1, err := summaries("preset.events", d)
	if err != nil {
		return nil, err
	}
	result = append(result, d1...)
	e, err := s.catalog.RuleSystems()
	if err != nil {
		return nil, err
	}
	e1, err := summaries("preset.rules", e)
	if err != nil {
		return nil, err
	}
	result = append(result, e1...)
	f, err := s.catalog.ActorStates()
	if err != nil {
		return nil, err
	}
	f1, err := summaries("preset.actor_state", f)
	if err != nil {
		return nil, err
	}
	result = append(result, f1...)
	refs, err := s.catalog.StyleReferences()
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if !ref.Missing {
			result = append(result, ExportResource{Local: LocalRef{Kind: "style.reference", Scope: "global", ID: filepath.Base(ref.Path)}, Name: ref.Name, Description: ref.Description})
		}
	}
	snapshot, err := s.catalog.SkillSnapshot(ctx, resourcecatalog.ProjectSkills(projectID))
	if err != nil {
		return nil, err
	}
	for _, skill := range snapshot.Skills {
		if skill.Scope == skills.ScopeBuiltin {
			continue
		}
		local := LocalRef{Kind: "skill", Scope: string(skill.Scope), ID: skill.Name}
		if skill.Scope == skills.ScopeWorkspace {
			local.ProjectID = projectID
		}
		result = append(result, ExportResource{Local: local, Name: skill.Name, Description: skill.Description})
	}
	for _, kind := range []platform.Kind{platform.Plugin, platform.Game} {
		items, err := s.platform.List(kind)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Removed {
				continue
			}
			for _, release := range item.Releases {
				if release.Digest == item.CurrentRelease {
					result = append(result, ExportResource{Local: LocalRef{Kind: "extension." + string(kind), Scope: "global", ID: item.ID}, Name: release.Manifest.Name.English})
				}
			}
		}
	}
	if projectID != "" {
		_, layout, err := s.registry.Resolve(projectID, true)
		if err != nil {
			return nil, err
		}
		items, err := lore.NewStore(layout.ContentRoot).ListAll()
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			result = append(result, ExportResource{Local: LocalRef{Kind: "lore.item", Scope: "project", ProjectID: projectID, ID: item.ID}, Name: item.Name, Description: item.BriefDescription})
		}
		snapshot, err := s.snapshot(ctx, FileTarget{ProjectID: projectID, Path: openingPath})
		if err != nil {
			return nil, err
		}
		if snapshot.Exists {
			var collection openings
			if err := json.Unmarshal(snapshot.Content, &collection); err != nil {
				return nil, err
			}
			for _, item := range collection.Presets {
				result = append(result, ExportResource{Local: LocalRef{Kind: "game.opening", Scope: "project", ProjectID: projectID, ID: item.ID}, Name: item.Title})
			}
		}
		snapshot, err = s.snapshot(ctx, FileTarget{ProjectID: projectID, Path: coverPath})
		if err != nil {
			return nil, err
		}
		if snapshot.Exists {
			result = append(result, ExportResource{Local: LocalRef{Kind: "project.cover", Scope: "project", ProjectID: projectID, ID: "cover"}, Name: "Cover"})
		}
	}
	return result, nil
}

var portableFields = map[string][]string{
	"preset.narrative":     {"name", "description", "style_refs", "style_rules", "context_policy", "slots"},
	"preset.image":         {"name", "description", "prompt", "slots"},
	"preset.game_planning": {"name", "description", "sections"},
	"preset.events":        {"name", "description", "events"},
	"preset.rules":         {"name", "description", "actor_state_id", "trpg_system"},
	"preset.actor_state":   {"name", "description", "actor_state"},
	"lore.item":            {"enabled", "type", "name", "importance", "tags", "brief_description", "keywords", "load_mode", "content"},
	"game.opening":         {"title", "content"},
}

func portableJSON(kind string, value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	for _, key := range portableFields[kind] {
		if field, ok := fields[key]; ok {
			result[key] = field
		}
	}
	return json.MarshalIndent(result, "", "  ")
}

func (s *Service) exportResource(ctx context.Context, ref LocalRef) (map[string][]byte, error) {
	if err := validateLocalRef(ref); err != nil {
		return nil, fmt.Errorf("invalid export resource")
	}
	var value any
	var err error
	switch ref.Kind {
	case "preset.narrative":
		value, err = s.catalog.Teller(ref.ID)
	case "preset.image":
		value, err = s.catalog.ImagePreset(ref.ID)
	case "preset.game_planning":
		value, err = s.catalog.GamePlanningTemplate(ref.ID)
	case "preset.events":
		value, err = s.catalog.EventPackage(ref.ID)
	case "preset.rules":
		value, err = s.catalog.RuleSystem(ref.ID)
	case "preset.actor_state":
		value, err = s.catalog.ActorState(ref.ID)
	case "style.reference":
		document, err := s.catalog.StyleReferenceFile(style.StoragePath(ref.ID))
		if err != nil {
			return nil, err
		}
		return map[string][]byte{"resource.md": []byte(document.Content)}, nil
	case "skill":
		snapshot, err := s.catalog.SkillSnapshot(ctx, resourcecatalog.ProjectSkills(ref.ProjectID))
		if err != nil {
			return nil, err
		}
		index := slices.IndexFunc(snapshot.Skills, func(skill skills.SkillSummary) bool { return skill.Name == ref.ID && string(skill.Scope) == ref.Scope })
		if index < 0 {
			return nil, fmt.Errorf("Skill not found")
		}
		file := snapshot.Skills[index].Path
		files, err := readFiles(filepath.Dir(file))
		if err != nil {
			return nil, err
		}
		delete(files, ".denova-source.json")
		return files, nil
	case "extension.plugin", "extension.game":
		kind := platform.Kind(strings.TrimPrefix(ref.Kind, "extension."))
		items, err := s.platform.List(kind)
		if err != nil {
			return nil, err
		}
		index := slices.IndexFunc(items, func(item platform.Installed) bool { return item.ID == ref.ID && !item.Removed })
		if index < 0 {
			return nil, fmt.Errorf("extension not found")
		}
		var buffer bytes.Buffer
		err = s.platform.ExportInstalled(platform.ReleaseRef{Package: platform.PackageRef{Kind: kind, ID: ref.ID}, ReleaseID: items[index].CurrentRelease}, &buffer)
		if err != nil {
			return nil, err
		}
		return platform.ArchiveFiles(buffer.Bytes())
	case "lore.item":
		_, layout, resolveErr := s.registry.Resolve(ref.ProjectID, true)
		if resolveErr != nil {
			return nil, resolveErr
		}
		item, readErr := lore.NewStore(layout.ContentRoot).ReadAny(ref.ID)
		if readErr != nil {
			return nil, readErr
		}
		raw, err := portableJSON(ref.Kind, item)
		if err != nil {
			return nil, err
		}
		return s.exportLoreMaterials(ctx, ref, item, raw)

	case "game.opening":
		snapshot, readErr := s.snapshot(ctx, FileTarget{ProjectID: ref.ProjectID, Path: openingPath})
		if readErr != nil {
			return nil, readErr
		}
		var collection openings
		if err = json.Unmarshal(snapshot.Content, &collection); err != nil {
			return nil, err
		}
		index := slices.IndexFunc(collection.Presets, func(item opening) bool { return item.ID == ref.ID })
		if index < 0 {
			return nil, fmt.Errorf("opening not found")
		}
		value = collection.Presets[index]
	case "project.cover":
		snapshot, err := s.snapshot(ctx, FileTarget{ProjectID: ref.ProjectID, Path: coverPath})
		if err != nil {
			return nil, err
		}
		if !snapshot.Exists {
			return nil, fmt.Errorf("cover missing")
		}
		raw, err := json.Marshal(portableImage{AssetPath: "image.png"})
		if err != nil {
			return nil, err
		}
		return map[string][]byte{"resource.json": raw, "image.png": snapshot.Content}, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err := portableJSON(ref.Kind, value)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{"resource.json": raw}, nil
}

func (s *Service) Export(ctx context.Context, request ExportRequest) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(request.Resources) == 0 || len(request.Resources) > 256 {
		return nil, fmt.Errorf("select between 1 and 256 resources")
	}
	if request.Native {
		if len(request.Resources) != 1 {
			return nil, fmt.Errorf("native export needs one resource")
		}
		ref := request.Resources[0]
		if ref.Kind != "skill" && !strings.HasPrefix(ref.Kind, "extension.") {
			return nil, fmt.Errorf("native export supports Skills and extensions")
		}
		files, err := s.exportResource(ctx, ref)
		if err != nil {
			return nil, err
		}
		return archiveBytes(files)
	}
	if request.Package.ID == "" {
		request.Package.ID = uuid.NewString()
	}
	if !resourceID.MatchString(request.Package.ID) || strings.TrimSpace(request.Package.Name) == "" {
		return nil, fmt.Errorf("package name and portable ID required")
	}
	resourceIDs := map[LocalRef]string{}
	if request.InstallationID != "" {
		installation, _, err := s.loadInstallation(ctx, request.InstallationID)
		if err != nil {
			return nil, err
		}
		if request.Package.ID == installation.Package.ID {
			for _, binding := range installation.Bindings {
				resourceIDs[binding.Local] = binding.ResourceID
			}
		}
	}
	platformState, err := s.platform.InstallState()
	if err != nil {
		return nil, err
	}
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: request.Package, Resources: []Resource{}}
	files := map[string][]byte{}
	seen := map[LocalRef]string{}
	var add func(LocalRef) (string, error)
	add = func(ref LocalRef) (string, error) {
		if id, ok := seen[ref]; ok {
			return id, nil
		}
		if len(seen) >= 256 {
			return "", fmt.Errorf("too many dependencies")
		}
		content, err := s.exportResource(ctx, ref)
		if err != nil {
			return "", err
		}
		identity, _ := json.Marshal(ref)
		id := resourceIDs[ref]
		if id == "" {
			id = uuid.NewSHA1(uuid.NameSpaceURL, append([]byte(request.Package.ID+":"), identity...)).String()
		}
		seen[ref] = id
		prefix := "resources/" + id
		resource := Resource{ID: id, Kind: ref.Kind, Path: prefix}
		if strings.HasPrefix(ref.Kind, "extension.") {
			file := "denova.plugin.json"
			if ref.Kind == "extension.game" {
				file = "denova.game.json"
			}
			var extension platform.Manifest
			if err := json.Unmarshal(content[file], &extension); err != nil {
				return "", err
			}
			dependencies, err := s.platform.ExportDependencies(extension)
			if err != nil {
				return "", err
			}
			for _, dependency := range dependencies {
				depID, err := add(LocalRef{Kind: "extension.plugin", Scope: "global", ID: dependency.PluginID})
				if err != nil {
					return "", err
				}
				resource.Requires = append(resource.Requires, depID)
			}
		}
		if raw, ok := content["resource.json"]; ok && !strings.HasPrefix(ref.Kind, "extension.") && ref.Kind != "skill" {
			resource.Path = path.Join(prefix, "resource.json")
			var body map[string]json.RawMessage
			if err := json.Unmarshal(raw, &body); err != nil {
				return "", err
			}
			if ref.Kind == "lore.item" {
				var materials portableMaterials
				if err := json.Unmarshal(body["materials"], &materials); err != nil {
					return "", err
				}
				for i := range materials.Entries {
					entry := &materials.Entries[i]
					if entry.URL != "" {
						continue
					}
					old := entry.AssetPath
					data, ok := content[old]
					if !ok {
						return "", fmt.Errorf("missing material payload")
					}
					name := path.Join("assets", path.Base(old))
					files[name] = data
					delete(content, old)
					entry.AssetPath = name
					resource.Assets = append(resource.Assets, name)
					if materials.CoverPath == old {
						materials.CoverPath = name
					}
				}
				body["materials"], _ = json.Marshal(materials)
			} else if ref.Kind == "project.cover" {
				var attachment portableImage
				if err := json.Unmarshal(raw, &attachment); err != nil {
					return "", err
				}
				attachment.AssetPath = path.Join(prefix, attachment.AssetPath)
				resource.Assets = []string{attachment.AssetPath}
				body["asset_path"], _ = json.Marshal(attachment.AssetPath)
			}

			dependency := func(kind, key string) (string, error) {
				dep := LocalRef{Kind: kind, Scope: "global", ID: key}
				depID, err := add(dep)
				if err != nil {
					return "", err
				}
				if !slices.Contains(resource.Requires, depID) {
					resource.Requires = append(resource.Requires, depID)
				}
				return depID, nil
			}
			if ref.Kind == "preset.rules" {
				var key string
				_ = json.Unmarshal(body["actor_state_id"], &key)
				if key != "" {
					id, err := dependency("preset.actor_state", key)
					if err != nil {
						return "", err
					}
					body["actor_state_ref"], _ = json.Marshal(id)
					delete(body, "actor_state_id")
				}
			}
			if ref.Kind == "preset.narrative" {
				rewrite := func(raw json.RawMessage) (json.RawMessage, error) {
					if len(raw) == 0 {
						return raw, nil
					}
					var keys []string
					if err := json.Unmarshal(raw, &keys); err != nil {
						return nil, err
					}
					for i, key := range keys {
						id, err := dependency("style.reference", filepath.Base(key))
						if err != nil {
							return nil, err
						}
						keys[i] = id
					}
					return json.Marshal(keys)
				}
				body["style_refs"], err = rewrite(body["style_refs"])
				if err != nil {
					return "", err
				}
				if len(body["style_refs"]) == 0 {
					delete(body, "style_refs")
				}
				if len(body["style_rules"]) > 0 {
					var rules []map[string]json.RawMessage
					if err := json.Unmarshal(body["style_rules"], &rules); err != nil {
						return "", err
					}
					for _, rule := range rules {
						rule["style_refs"], err = rewrite(rule["style_refs"])
						if err != nil {
							return "", err
						}
						if len(rule["style_refs"]) == 0 {
							delete(rule, "style_refs")
						}
					}
					body["style_rules"], _ = json.Marshal(rules)
				}
			}
			content["resource.json"], err = json.MarshalIndent(body, "", "  ")
			if err != nil {
				return "", err
			}
		} else if ref.Kind == "style.reference" {
			resource.Path = path.Join(prefix, "resource.md")
		} else if ref.Kind == "project.cover" {
			resource.Path = path.Join(prefix, "resource.png")
		}
		for name, raw := range content {
			files[path.Join(prefix, name)] = raw
		}
		manifest.Resources = append(manifest.Resources, resource)
		return id, nil
	}
	for _, ref := range request.Resources {
		if _, err := add(ref); err != nil {
			return nil, err
		}
	}
	currentState, err := s.platform.InstallState()
	if err != nil {
		return nil, err
	}
	if currentState != platformState {
		return nil, fmt.Errorf("extensions changed during export; preview again")
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files["denova-pack.json"] = raw
	// Validate the final archive with the same bounded admission as import.
	result, err := archiveBytes(files)
	if err != nil {
		return nil, err
	}
	if _, err := platform.ArchiveFiles(result); err != nil {
		return nil, err
	}
	return result, nil
}
