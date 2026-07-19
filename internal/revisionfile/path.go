package revisionfile

import (
	"os"
	"path/filepath"
)

// canonicalPath resolves the longest existing prefix so symlink aliases share
// one lock even when the target file or one of its child directories is new.
func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	abs = filepath.Clean(abs)
	prefix := abs
	suffix := []string{}
	for {
		if _, statErr := os.Lstat(prefix); statErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(prefix); resolveErr == nil {
				for index := len(suffix) - 1; index >= 0; index-- {
					resolved = filepath.Join(resolved, suffix[index])
				}
				return filepath.Clean(resolved)
			}
			break
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			break
		}
		suffix = append(suffix, filepath.Base(prefix))
		prefix = parent
	}
	return abs
}
