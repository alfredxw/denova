package app

import (
	"path/filepath"
	"strings"
)

func (s *AutomationAppService) chapterContentMutationPaths(snap *automationWorkspaceSnapshot, paths []string) []string {
	workspace := snap.workspace
	seen := map[string]bool{}
	targets := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) && strings.TrimSpace(workspace) != "" {
			rel, err := filepath.Rel(workspace, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = rel
			} else {
				canonicalWorkspace := canonicalAutomationWorkspace(workspace)
				canonicalPath := path
				if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
					canonicalPath = resolved
				}
				if rel, relErr := filepath.Rel(canonicalWorkspace, canonicalPath); relErr == nil {
					path = rel
				}
			}
		}
		path = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(path), "./"))
		if !isChapterContentMutationPath(path) || seen[path] {
			continue
		}
		seen[path] = true
		targets = append(targets, path)
	}
	return targets
}

func isChapterContentMutationPath(path string) bool {
	path = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(path), "./"))
	if !strings.HasPrefix(path, "chapters/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}
