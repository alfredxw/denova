package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const workspaceDirectoryReadBatch = 256

type workspaceScanBudget struct {
	entries int
}

// readWorkspaceDirectory reads in batches so an attacker-controlled directory
// cannot force fs.ReadDir to allocate for an unbounded number of names before
// policy is consulted. Sorting remains deterministic inside the explicit scan
// budget; larger trees fail closed and ask the caller to narrow the path.
func readWorkspaceDirectory(
	ctx context.Context,
	root *os.Root,
	directory string,
	budget *workspaceScanBudget,
) ([]fs.DirEntry, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	file, err := root.Open(filepath.FromSlash(directory))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries := make([]fs.DirEntry, 0, workspaceDirectoryReadBatch)
	for {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		batch, readErr := file.ReadDir(workspaceDirectoryReadBatch)
		if len(batch) > 0 {
			budget.entries += len(batch)
			if budget.entries > maxWorkspaceScanEntries {
				return nil, fmt.Errorf("workspace directory scan exceeds %d entries; narrow the path", maxWorkspaceScanEntries)
			}
			entries = append(entries, batch...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
