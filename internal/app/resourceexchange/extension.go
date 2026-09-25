package resourceexchange

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"denova/internal/platform"
)

// PreviewExtension adopts an already-frozen platform candidate, for developer
// publishing and existing source pickers. It never downloads a second copy.
func (s *Service) PreviewExtension(ctx context.Context, id string) (Preview, error) {
	candidate, err := s.platform.CandidateInfo(id)
	if err != nil {
		return Preview{}, err
	}
	var buffer bytes.Buffer
	if err := s.platform.ExportCandidate(id, &buffer); err != nil {
		return Preview{}, err
	}
	files, err := platform.ArchiveFiles(buffer.Bytes())
	if err != nil {
		return Preview{}, err
	}
	source := Source{Kind: "file", Filename: candidate.Manifest.ID + ".zip"}
	if candidate.Source != nil {
		origin := candidate.Source
		source = Source{Kind: "github", URL: origin.URL, Ref: origin.Ref, Path: origin.Path, Commit: origin.Commit}
		if source.Path == "." {
			source.Path = ""
		}
	}
	return s.previewFiles(ctx, source, files)
}

// PlanExtension uses the same single-owner boundary as mixed resource bundles.
// An editor cannot independently replace a member owned by a tracked bundle.
func (s *Service) PlanExtension(ctx context.Context, id string, grants []string) (Plan, error) {
	preview, err := s.PreviewExtension(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	candidate := preview.Candidates[0]
	resource := candidate.Resources[0]
	items, err := s.Installations(ctx)
	if err != nil {
		return Plan{}, err
	}
	request := PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: []string{resource.ID}, Grants: map[string][]string{resource.ID: grants}}
	for _, item := range items {
		if item.Tracking != "tracked" {
			continue
		}
		if slices.ContainsFunc(item.Bindings, func(binding Binding) bool {
			return binding.Local.Kind == resource.Kind && binding.Local.ID == resource.Extension.Manifest.ID && binding.Ownership == "owned"
		}) {
			if len(item.Bindings) != 1 {
				return Plan{}, ErrBundleOwned
			}
			request.InstallationID = item.ID
			request.UpdateMode = item.UpdateMode
			request.Resources = []string{item.Bindings[0].ResourceID}
			if item.Bindings[0].ResourceID != resource.ID {
				return Plan{}, fmt.Errorf("extension binding differs; update its original installation")
			}
			break
		}
	}
	return s.Plan(ctx, request)
}

func (s *Service) ExtensionOwner(ctx context.Context, ref platform.PackageRef) (Installation, error) {
	items, err := s.Installations(ctx)
	if err != nil {
		return Installation{}, err
	}
	for _, item := range items {
		if item.Tracking != "tracked" {
			continue
		}
		for _, binding := range item.Bindings {
			if binding.Ownership == "owned" && binding.Local.Kind == "extension."+string(ref.Kind) && binding.Local.ID == ref.ID {
				return item, nil
			}
		}
	}
	return Installation{}, fmt.Errorf("extension has no tracked installation")
}

func (s *Service) ExtensionCatalog(ctx context.Context) ([]platform.CatalogEntry, error) {
	entries, err := s.platform.Catalog()
	if err != nil {
		return nil, err
	}
	items, err := s.installations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		for _, item := range items {
			if item.Tracking != "tracked" || item.Source.Kind != "github" {
				continue
			}
			for _, binding := range item.Bindings {
				if binding.Ownership == "owned" && binding.Local.Kind == "extension."+string(entries[i].Kind) && binding.Local.ID == entries[i].ID {
					source := item.Source
					entries[i].Source = &platform.GitHubSource{URL: source.URL, Ref: source.Ref, Path: source.Path, Commit: source.Commit}
				}
			}
		}
	}
	return entries, nil
}

func (s *Service) CheckExtensionUpdate(ctx context.Context, ref platform.PackageRef) (platform.GitHubUpdate, error) {
	item, err := s.ExtensionOwner(ctx, ref)
	if err != nil {
		return platform.GitHubUpdate{}, err
	}
	preview, err := s.CheckUpdate(ctx, item.ID)
	if err != nil {
		return platform.GitHubUpdate{}, err
	}
	defer s.DiscardPreview(preview.ID)
	current, _, err := s.loadInstallation(ctx, item.ID)
	if err != nil {
		return platform.GitHubUpdate{}, err
	}
	status := "current"
	if current.RemoteState == "update_available" {
		status = "available"
	} else if current.RemoteState != "unchanged" {
		return platform.GitHubUpdate{}, fmt.Errorf("upstream package identity changed")
	}
	source := preview.Source
	return platform.GitHubUpdate{Status: status, Source: platform.GitHubSource{URL: source.URL, Ref: source.Ref, Path: source.Path, Commit: source.Commit}}, nil
}
func (s *Service) PreviewExtensionUpdate(ctx context.Context, ref platform.PackageRef, commit string) (platform.Candidate, error) {
	item, err := s.ExtensionOwner(ctx, ref)
	if err != nil {
		return platform.Candidate{}, err
	}
	if len(item.Bindings) != 1 {
		return platform.Candidate{}, fmt.Errorf("update this extension through its resource bundle")
	}
	source := item.Source
	if source.Kind != "github" {
		return platform.Candidate{}, fmt.Errorf("extension has no GitHub source")
	}
	candidate, err := s.platform.PreviewGitHub(ctx, platform.GitHubSource{URL: source.URL, Ref: source.Ref, Path: source.Path, Commit: commit})
	if err != nil {
		return candidate, err
	}
	if candidate.Kind != ref.Kind || candidate.Manifest.ID != ref.ID {
		s.platform.DiscardCandidate(candidate.ID)
		return platform.Candidate{}, fmt.Errorf("upstream extension identity changed")
	}
	return candidate, nil
}
