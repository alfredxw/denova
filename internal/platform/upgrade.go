package platform

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"denova/internal/portablepath"
	"github.com/google/uuid"
)

// checkUpgrade starts the target backend against a disposable save copy. The
// canonical instance binding and data remain intact when startup fails.
func (m *Manager) checkUpgrade(ctx context.Context, instance Instance, release Release, pins []DependencyPin) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := ".upgrade-" + uuid.NewString()
	directory := m.instancePath(instance.GameID, id)
	defer os.RemoveAll(directory)
	source := filepath.Join(m.instancePath(instance.GameID, instance.ID), "data")
	if err := portablepath.PreflightTree(source); err != nil {
		return err
	}
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(directory, "data", relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeBytes(target, data)
	}); err != nil {
		return err
	}
	runtime, err := m.startRuntime(id, release, Scope{Kind: "game-instance", InstanceID: id, ProjectID: instance.ProjectID}, pins, RuntimeConfiguration{Setup: instance.Setup}, instance.Models, true, OpenOptions{ParentOrigin: "http://127.0.0.1"})
	if err != nil {
		return err
	}
	return runtime.close(ctx)
}
