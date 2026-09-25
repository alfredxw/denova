package resourceexchange

import (
	"context"
	"os"
	"path"
	"strings"

	"denova/internal/revisionfile"
)

// Include added files in the local modification check; a new script is a local
// change even when all upstream-tracked files still match their baselines.
func (s *Service) skillLocalState(ctx context.Context, binding Binding) (string, error) {
	prefix := path.Join("skills", binding.Local.ID)
	directory, err := resolveTarget(s.root, s.registry, FileTarget{ProjectID: binding.Local.ProjectID, Path: prefix})
	if err != nil {
		return "unavailable", err
	}
	files, err := readFiles(directory)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "unavailable", err
	}
	for name, content := range files {
		if strings.HasPrefix(name, ".denova-locks/") {
			continue
		}
		if binding.Baseline[path.Join(prefix, name)] != revisionfile.Revision(content) {
			return "modified", nil
		}
	}
	for name := range binding.Baseline {
		if _, ok := files[strings.TrimPrefix(name, prefix+"/")]; !ok {
			return "missing", nil
		}
	}
	return "unchanged", ctx.Err()
}
