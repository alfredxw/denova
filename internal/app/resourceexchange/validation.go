package resourceexchange

import (
	"denova/internal/portablepath"
	"denova/internal/project"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// inferReferences considers only typed fields. Free-form prose and scripts never
// create dependencies or cause implicit downloads.
func inferReferences(dir string, candidate *PackagePreview) error {
	identities := map[string]string{}
	for _, resource := range candidate.Resources {
		for _, key := range []string{resource.ID, resource.Path} {
			identities[resource.Kind+":"+key] = resource.ID
		}
	}
	for i := range candidate.Resources {
		resource := &candidate.Resources[i]
		if resource.Kind != "preset.narrative" && resource.Kind != "preset.rules" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(resource.Path)))
		if err != nil {
			return err
		}
		var fields struct {
			StyleRefs  []string `json:"style_refs"`
			StyleRules []struct {
				StyleRefs []string `json:"style_refs"`
			} `json:"style_rules"`
			ActorStateRef string `json:"actor_state_ref"`
			ActorStateID  string `json:"actor_state_id"`
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		dependencies := map[string]string{}
		if resource.Kind == "preset.narrative" {
			for _, ref := range fields.StyleRefs {
				dependencies[ref] = "style.reference"
			}
			for _, rule := range fields.StyleRules {
				for _, ref := range rule.StyleRefs {
					dependencies[ref] = "style.reference"
				}
			}
		} else {
			ref := fields.ActorStateRef
			if ref == "" {
				ref = fields.ActorStateID
			}
			if ref != "" {
				dependencies[ref] = "preset.actor_state"
			}
		}
		for key, kind := range dependencies {
			id, ok := identities[kind+":"+key]
			if !ok {
				return fmt.Errorf("missing typed dependency %s", key)
			}
			if !slices.Contains(resource.Requires, id) {
				resource.Requires = append(resource.Requires, id)
			}
		}
	}
	return nil
}

func validateResourcePaths(resources []Resource) error {
	paths := map[string]string{}
	for _, resource := range resources {
		name := strings.ToLower(resource.Path)
		if previous, ok := paths[name]; ok {
			return fmt.Errorf("resources %s and %s share a path", previous, resource.ID)
		}
		paths[name] = resource.ID
		directory := resource.Kind == "skill" || strings.HasPrefix(resource.Kind, "extension.")
		if resource.Path == "." && !directory {
			return fmt.Errorf("resource requires a file path")
		}
		if directory {
			for _, other := range resources {
				if other.ID != resource.ID && (resource.Path == "." || strings.HasPrefix(strings.ToLower(other.Path), name+"/")) {
					return fmt.Errorf("resource directories overlap")
				}
			}
		}
		for _, asset := range resource.Assets {
			if path.Base(asset) == "denova-pack.json" {
				return fmt.Errorf("manifest cannot be an asset")
			}
		}
	}
	return nil
}

func validateLocalRef(ref LocalRef) error {
	if !validKind(ref.Kind) || portablepath.ValidateComponent(ref.ID) != nil {
		return fmt.Errorf("invalid resource reference")
	}
	if ref.ProjectID != "" {
		if err := project.ValidateID(ref.ProjectID); err != nil {
			return err
		}
	}
	switch ref.Kind {
	case "skill":
		if ref.Scope != "builtin" && ref.Scope != "user" && ref.Scope != "workspace" && ref.Scope != "shared" {
			return fmt.Errorf("invalid Skill scope")
		}
		if ref.Scope == "workspace" && ref.ProjectID == "" {
			return fmt.Errorf("workspace Skill requires Project")
		}
		if ref.Scope != "workspace" && ref.ProjectID != "" {
			return fmt.Errorf("global Skill cannot have Project identity")
		}
	case "lore.item", "game.opening", "project.cover":
		if ref.Scope != "project" || ref.ProjectID == "" {
			return fmt.Errorf("resource requires Project scope")
		}
	default:
		if ref.Scope != "global" || ref.ProjectID != "" {
			return fmt.Errorf("resource requires global scope")
		}
	}
	return nil
}

// Native resource payloads expose portable fields only. Runtime paths, source
// receipts and model/provider configuration cannot ride along with definitions.
func validatePayload(kind string, raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	allowed := slices.Clone(portableFields[kind])
	switch kind {
	case "preset.rules":
		allowed = append(allowed, "actor_state_ref")
	case "lore.item":
		allowed = append(allowed, "image", "materials")
	case "project.cover":
		allowed = []string{"asset_path", "alt_text"}
	}
	for key := range fields {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("non-portable field %s in %s", key, kind)
		}
	}
	return nil
}
