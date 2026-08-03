package projectfiles

import (
	"context"
	"log/slog"
	"os"
)

func normalizeTreeEntryLimit(limit int) int {
	if limit <= 0 {
		return DefaultTreeEntryLimit
	}
	if limit > MaximumTreeEntryLimit {
		return MaximumTreeEntryLimit
	}
	return limit
}

func resolveTreeEntryBudget(requested, configuredLimit int) int {
	limit := normalizeTreeEntryLimit(configuredLimit)
	if requested <= 0 || requested > limit {
		return limit
	}
	return requested
}

type pendingTreeDirectory struct {
	path   string
	cursor string
	root   bool
}

// resolveTreeTargetRecursive uses breadth-first traversal so a request that
// reaches its high entry ceiling leaves a coherent shallow tree instead of
// fully loading one arbitrary deep branch. Nested read failures stay isolated:
// the successfully resolved pages remain usable and the failed directory can
// still surface its own error if the user explicitly expands it later.
func resolveTreeTargetRecursive(
	ctx context.Context,
	root *os.Root,
	result TreeResolveResult,
	path string,
	cursor string,
	includeIgnored bool,
	budget int,
) (TreeResolveResult, int) {
	queue := []pendingTreeDirectory{{path: path, cursor: cursor, root: true}}
	queued := map[string]struct{}{path: {}}
	used := 0
	for len(queue) > 0 && budget > 0 {
		current := queue[0]
		queue = queue[1:]
		page, err := readDirectoryPage(ctx, root, current.path, includeIgnored, current.cursor, budget)
		if err != nil {
			if current.root {
				result.Code = treeResolveErrorCode(err)
				result.Error = err.Error()
				return result, used
			}
			slog.WarnContext(ctx, "[internal/app/projectfiles/tree_recursive.go] recursive project file tree skipped an unreadable descendant", "target_path", result.Path, "directory_path", current.path, "error", err)
			continue
		}
		result.Directories = append(result.Directories, page)
		used += len(page.Entries)
		budget -= len(page.Entries)
		for _, entry := range page.Entries {
			if entry.Type != EntryDirectory || entry.Symlink {
				continue
			}
			if _, exists := queued[entry.Path]; exists {
				continue
			}
			queued[entry.Path] = struct{}{}
			queue = append(queue, pendingTreeDirectory{path: entry.Path})
		}
	}
	result.OK = true
	return result, used
}

func recursiveTreeResultComplete(result TreeResolveResult) bool {
	if !result.OK {
		return false
	}
	loaded := make(map[string]struct{}, len(result.Directories))
	for _, directory := range result.Directories {
		loaded[directory.Path] = struct{}{}
		if directory.ChildrenState != DirectoryChildrenComplete {
			return false
		}
	}
	for _, directory := range result.Directories {
		for _, entry := range directory.Entries {
			if entry.Type != EntryDirectory || entry.Symlink {
				continue
			}
			if _, ok := loaded[entry.Path]; !ok {
				return false
			}
		}
	}
	return true
}
