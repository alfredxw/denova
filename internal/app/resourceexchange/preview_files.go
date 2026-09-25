package resourceexchange

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type PreviewFile struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}
type ResourceFiles struct {
	Files     []PreviewFile `json:"files"`
	Path      string        `json:"path,omitempty"`
	Content   string        `json:"content,omitempty"`
	Truncated bool          `json:"truncated"`
	Binary    bool          `json:"binary"`
}

// PreviewFiles exposes only the resource's frozen files. Query paths are map
// keys, never paths supplied to the host filesystem. Text previews cap at 64 KiB.
func (s *Service) PreviewFiles(ctx context.Context, previewID, candidateID, resourceID, selected string) (ResourceFiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	preview, dir, err := s.loadPreview(previewID)
	if err != nil {
		return ResourceFiles{}, err
	}
	var resource *PreviewResource
	for _, candidate := range preview.Candidates {
		if candidate.ID != candidateID {
			continue
		}
		for i := range candidate.Resources {
			if candidate.Resources[i].ID == resourceID {
				resource = &candidate.Resources[i]
				break
			}
		}
	}
	if resource == nil {
		return ResourceFiles{}, fmt.Errorf("resource candidate unavailable")
	}
	files := map[string][]byte{}
	location := filepath.Join(dir, "files", filepath.FromSlash(resource.Path))
	if resource.Kind == "skill" || strings.HasPrefix(resource.Kind, "extension.") {
		files, err = readFiles(location)
		if err != nil {
			return ResourceFiles{}, err
		}
	} else {
		raw, err := os.ReadFile(location)
		if err != nil {
			return ResourceFiles{}, err
		}
		files[resource.Path] = raw
	}
	result := ResourceFiles{Files: []PreviewFile{}}
	for name, raw := range files {
		result.Files = append(result.Files, PreviewFile{Path: name, Bytes: len(raw)})
	}
	slices.SortFunc(result.Files, func(a, b PreviewFile) int { return strings.Compare(a.Path, b.Path) })
	if selected != "" {
		raw, ok := files[selected]
		if !ok {
			return ResourceFiles{}, fmt.Errorf("file not in resource")
		}
		result.Path = selected
		result.Binary = !utf8.Valid(raw) || strings.ContainsRune(string(raw), '\x00')
		if !result.Binary {
			if len(raw) > 64*1024 {
				result.Truncated = true
				raw = raw[:64*1024]
				for !utf8.Valid(raw) {
					raw = raw[:len(raw)-1]
				}
			}
			result.Content = string(raw)
		}
	}
	return result, ctx.Err()
}
