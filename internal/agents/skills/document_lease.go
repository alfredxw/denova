package skills

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"denova/internal/filelease"
)

type skillLeaseTarget struct {
	dir  Directory
	name string
}

func withSkillLease[T any](ctx context.Context, dir Directory, name string, operation func() (T, error)) (T, error) {
	return withSkillLeases(ctx, []skillLeaseTarget{{dir: dir, name: name}}, operation)
}

func withSkillLeases[T any](ctx context.Context, targets []skillLeaseTarget, operation func() (T, error)) (result T, err error) {
	paths := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if err := ValidateName(target.name); err != nil {
			return result, err
		}
		lockPath := filepath.Join(target.dir.Path, ".denova-locks", "skill-mutations", target.name+".lock")
		if !seen[lockPath] {
			seen[lockPath] = true
			paths = append(paths, lockPath)
		}
	}
	sort.Strings(paths)
	releases := make([]func() error, 0, len(paths))
	for _, lockPath := range paths {
		release, acquireErr := filelease.Acquire(ctx, lockPath)
		if acquireErr != nil {
			for index := len(releases) - 1; index >= 0; index-- {
				acquireErr = errors.Join(acquireErr, releases[index]())
			}
			return result, fmt.Errorf("acquire skill mutation lease: %w", acquireErr)
		}
		releases = append(releases, release)
	}
	defer func() {
		for index := len(releases) - 1; index >= 0; index-- {
			if releaseErr := releases[index](); releaseErr != nil {
				err = errors.Join(err, releaseErr)
			}
		}
	}()
	return operation()
}
