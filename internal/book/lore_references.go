package book

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const directorLoreContextFilename = "lore-context.md"

type loreReferenceRewrite struct {
	path   string
	before []byte
	after  []byte
}

func validateLoreReferenceName(name string) error {
	if strings.Contains(name, "[[") || strings.Contains(name, "]]") {
		return fmt.Errorf("资料名称不能包含引用保留符号 [[ 或 ]]")
	}
	return nil
}

func prepareLoreReferenceRewrites(workspace, oldName, newName string) ([]loreReferenceRewrite, error) {
	oldRef := "[[" + strings.TrimSpace(oldName) + "]]"
	newRef := "[[" + strings.TrimSpace(newName) + "]]"
	if oldRef == newRef {
		return nil, nil
	}
	root := filepath.Join(workspace, "interactive", "stories")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	result := []loreReferenceRewrite{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != directorLoreContextFilename {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(data), oldRef, newRef)
		if updated == string(data) {
			return nil
		}
		result = append(result, loreReferenceRewrite{path: path, before: data, after: []byte(updated)})
		return nil
	})
	return result, err
}

func applyLoreReferenceRewrites(rewrites []loreReferenceRewrite) error {
	applied := 0
	for idx, rewrite := range rewrites {
		if err := os.WriteFile(rewrite.path, rewrite.after, 0o644); err != nil {
			for rollback := applied - 1; rollback >= 0; rollback-- {
				_ = os.WriteFile(rewrites[rollback].path, rewrites[rollback].before, 0o644)
			}
			return fmt.Errorf("更新资料引用失败 path=%s index=%d: %w", rewrite.path, idx, err)
		}
		applied++
	}
	return nil
}

func loreReferencePaths(workspace, name string) ([]string, error) {
	rewrites, err := prepareLoreReferenceRewrites(workspace, name, name+"__reference_probe__")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(rewrites))
	for _, rewrite := range rewrites {
		paths = append(paths, filepath.ToSlash(rewrite.path))
	}
	return paths, nil
}
