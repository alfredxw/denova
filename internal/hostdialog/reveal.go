package hostdialog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// PathRevealer opens the host file manager at an existing absolute path.
type PathRevealer interface {
	RevealPath(context.Context, string) error
}

type nativePathRevealer struct{}

func NewPathRevealer() PathRevealer {
	return nativePathRevealer{}
}

func (nativePathRevealer) RevealPath(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("host reveal path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect host reveal path: %w", err)
	}
	name, args, err := revealCommand(runtime.GOOS, filepath.Clean(path), info.IsDir())
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, name)
	}
	if err := exec.CommandContext(ctx, name, args...).Run(); err != nil {
		return fmt.Errorf("reveal path in host file manager: %w", err)
	}
	return nil
}

func revealCommand(goos, path string, directory bool) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}, nil
	case "windows":
		return "explorer.exe", []string{"/select,", path}, nil
	case "linux":
		if !directory {
			path = filepath.Dir(path)
		}
		return "xdg-open", []string{path}, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported host platform %s", ErrUnavailable, goos)
	}
}
