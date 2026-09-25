package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"denova/internal/platform"
	"denova/internal/portablepath"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

// ExportDefinition records a reusable selection, never another copy of content.
// Revision is an API precondition and is not part of the persisted definition.
type ExportDefinition struct {
	ExportRequest
	Revision string `json:"revision,omitempty"`
}

type ExportPlan struct {
	ID        string      `json:"export_id"`
	Package   PackageInfo `json:"package"`
	Resources []Resource  `json:"resources"`
	Files     int         `json:"files"`
	Bytes     int         `json:"bytes"`
	ExpiresAt time.Time   `json:"expires_at"`
}

func (s *Service) exportDefinitionPath(id string) (string, error) {
	if !resourceID.MatchString(id) || portablepath.ValidateComponent(id) != nil {
		return "", fmt.Errorf("invalid export definition ID")
	}
	relative := "resource-exchange/exports/definitions/" + id + ".json"
	return resolveTarget(s.root, s.registry, FileTarget{Path: relative})
}

func (s *Service) ExportDefinitions(ctx context.Context) ([]ExportDefinition, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "resource-exchange", "exports", "definitions"))
	if os.IsNotExist(err) {
		return []ExportDefinition{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []ExportDefinition{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		snapshot, err := revisionfile.Read(ctx, filepath.Join(s.root, "resource-exchange", "exports", "definitions", entry.Name()))
		if err != nil {
			return nil, err
		}
		var item ExportDefinition
		if err := json.Unmarshal(snapshot.Content, &item); err != nil {
			return nil, err
		}
		item.Revision = snapshot.Revision
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) SaveExportDefinition(ctx context.Context, definition ExportDefinition) (ExportDefinition, error) {
	file, err := s.exportDefinitionPath(definition.Package.ID)
	if err != nil {
		return ExportDefinition{}, err
	}
	if len(definition.Resources) == 0 || len(definition.Resources) > 256 {
		return ExportDefinition{}, fmt.Errorf("select between 1 and 256 resources")
	}
	for _, ref := range definition.Resources {
		if err := validateLocalRef(ref); err != nil {
			return ExportDefinition{}, fmt.Errorf("invalid resource selection")
		}
	}
	expected := definition.Revision
	if expected == "" {
		expected = revisionfile.MissingRevision
	}
	definition.Revision = ""
	raw, err := json.Marshal(definition)
	if err != nil {
		return ExportDefinition{}, err
	}
	snapshot, err := revisionfile.ReplaceIfRevision(ctx, file, expected, raw, revisionfile.Options{})
	definition.Revision = snapshot.Revision
	return definition, err
}

func (s *Service) DeleteExportDefinition(ctx context.Context, id, revision string) error {
	file, err := s.exportDefinitionPath(id)
	if err != nil {
		return err
	}
	if revision == "" {
		return fmt.Errorf("definition revision required")
	}
	return revisionfile.WithFiles(ctx, []string{file}, func(files *revisionfile.LockedFiles) error {
		current, err := files.Snapshot(file)
		if err != nil {
			return err
		}
		if current.Revision != revision {
			return &revisionfile.ConflictError{Path: file, Expected: revision, Actual: current.Revision}
		}
		return files.Replace(file, nil, false)
	})
}

// PrepareExport freezes exact distributable bytes before the user downloads.
// Editing a library after this call cannot silently change the reviewed archive.
func (s *Service) PrepareExport(ctx context.Context, request ExportRequest) (ExportPlan, error) {
	raw, err := s.Export(ctx, request)
	if err != nil {
		return ExportPlan{}, err
	}
	files, err := platform.ArchiveFiles(raw)
	if err != nil {
		return ExportPlan{}, err
	}
	plan := ExportPlan{ID: uuid.NewString(), Package: request.Package, Resources: []Resource{}, Files: len(files), Bytes: len(raw), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if manifest, ok := files["denova-pack.json"]; ok {
		var value Manifest
		if err := json.Unmarshal(manifest, &value); err != nil {
			return ExportPlan{}, err
		}
		plan.Package, plan.Resources = value.Package, value.Resources
	}
	base := filepath.Join(s.root, "resource-exchange", "exports", "previews")
	if err := os.MkdirAll(base, 0700); err != nil {
		return ExportPlan{}, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return ExportPlan{}, err
	}
	for _, entry := range entries {
		if _, err := uuid.Parse(entry.Name()); err != nil {
			continue
		}
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
			_ = os.RemoveAll(filepath.Join(base, entry.Name()))
		}
	}
	metadata, err := json.Marshal(plan)
	if err != nil {
		return ExportPlan{}, err
	}
	if err := writeFiles(filepath.Join(base, plan.ID), map[string][]byte{"package.zip": raw, "preview.json": metadata}); err != nil {
		return ExportPlan{}, err
	}
	return plan, nil
}

func (s *Service) ReadExport(ctx context.Context, id string) ([]byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, "resource-exchange", "exports", "previews", id)
	meta, err := revisionfile.Read(ctx, filepath.Join(dir, "preview.json"))
	if err != nil {
		return nil, err
	}
	var plan ExportPlan
	if err := json.Unmarshal(meta.Content, &plan); err != nil {
		return nil, err
	}
	if time.Now().After(plan.ExpiresAt) {
		return nil, fmt.Errorf("export preview expired")
	}
	snapshot, err := revisionfile.Read(ctx, filepath.Join(dir, "package.zip"))
	return slices.Clone(snapshot.Content), err
}
