package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/filelease"
)

func configResourceLeasePath(novaDir, resource string) string {
	novaDir = strings.TrimSpace(novaDir)
	if novaDir == "" {
		return ""
	}
	return filepath.Join(novaDir, ".locks", "config-resources", resource+".lock")
}

func withConfigResourceLease(ctx context.Context, lockPath string, operation func() (any, error)) (result any, err error) {
	if strings.TrimSpace(lockPath) == "" {
		return nil, fmt.Errorf("config resource storage directory is required")
	}
	release, err := filelease.Acquire(ctx, lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquire config resource lease: %w", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}()
	return operation()
}
