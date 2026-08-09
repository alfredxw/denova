package filewatch

import (
	"path/filepath"
	"strings"
)

// changeCoalescer applies the same important reductions as VS Code's watcher:
// create+delete disappears, delete+create becomes update, and create+write
// remains create. This also turns atomic file replacement into one update.
type changeCoalescer struct {
	changes map[string]Change
	order   []string
}

func newChangeCoalescer() *changeCoalescer {
	return &changeCoalescer{changes: make(map[string]Change)}
}

func (c *changeCoalescer) add(change Change) {
	path := normalizeRelativePath(change.Path)
	if path == "" {
		return
	}
	change.Path = path
	previous, exists := c.changes[path]
	if !exists {
		c.changes[path] = change
		c.order = append(c.order, path)
		return
	}

	nextType, keep := mergeChangeTypes(previous.Type, change.Type)
	if !keep {
		delete(c.changes, path)
		return
	}
	previous.Type = nextType
	c.changes[path] = previous
}

func (c *changeCoalescer) take() []Change {
	if len(c.changes) == 0 {
		c.order = c.order[:0]
		return nil
	}
	result := make([]Change, 0, len(c.changes))
	emitted := make(map[string]struct{}, len(c.changes))
	for _, path := range c.order {
		change, ok := c.changes[path]
		if !ok {
			continue
		}
		if _, duplicate := emitted[path]; duplicate {
			continue
		}
		if change.Type == ChangeDeleted && c.hasDeletedParent(path) {
			continue
		}
		result = append(result, change)
		emitted[path] = struct{}{}
	}
	c.changes = make(map[string]Change)
	c.order = c.order[:0]
	return result
}

func (c *changeCoalescer) hasDeletedParent(path string) bool {
	for {
		separator := strings.LastIndexByte(path, '/')
		if separator < 0 {
			return false
		}
		path = path[:separator]
		if change, exists := c.changes[path]; exists && change.Type == ChangeDeleted {
			return true
		}
	}
}

func mergeChangeTypes(previous, next ChangeType) (ChangeType, bool) {
	switch previous {
	case ChangeAdded:
		if next == ChangeDeleted {
			return "", false
		}
		return ChangeAdded, true
	case ChangeUpdated:
		if next == ChangeDeleted {
			return ChangeDeleted, true
		}
		return ChangeUpdated, true
	case ChangeDeleted:
		if next == ChangeDeleted {
			return ChangeDeleted, true
		}
		return ChangeUpdated, true
	default:
		return next, true
	}
}

func normalizeRelativePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	path = strings.TrimPrefix(path, "./")
	if path == "" || path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return ""
	}
	return path
}
