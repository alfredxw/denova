// Package hostdialog exposes small, explicitly user-triggered native UI effects.
// It belongs at the HTTP host boundary: project domain code never opens windows.
package hostdialog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"
)

var ErrUnavailable = errors.New("native directory picker is unavailable")

// DirectorySelection distinguishes a deliberate cancellation from a host error.
type DirectorySelection struct {
	Path     string `json:"path"`
	Canceled bool   `json:"canceled"`
}

type DirectoryOptions struct {
	Title       string
	InitialPath string
}

type DirectoryPicker interface {
	SelectDirectory(context.Context, DirectoryOptions) (DirectorySelection, error)
}

type nativeDirectoryPicker struct {
	gate chan struct{}
}

func NewDirectoryPicker() DirectoryPicker {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &nativeDirectoryPicker{gate: gate}
}

func (picker *nativeDirectoryPicker) SelectDirectory(ctx context.Context, options DirectoryOptions) (DirectorySelection, error) {
	if !zenity.IsAvailable() {
		return DirectorySelection{}, ErrUnavailable
	}
	select {
	case <-ctx.Done():
		return DirectorySelection{}, ctx.Err()
	case <-picker.gate:
	}
	defer func() { picker.gate <- struct{}{} }()

	dialogOptions := []zenity.Option{
		zenity.Directory(),
		zenity.Context(ctx),
	}
	if title := strings.TrimSpace(options.Title); title != "" {
		dialogOptions = append(dialogOptions, zenity.Title(title))
	}
	if initial := nearestExistingDirectory(options.InitialPath); initial != "" {
		dialogOptions = append(dialogOptions, zenity.Filename(initial+string(filepath.Separator)))
	}

	selected, err := zenity.SelectFile(dialogOptions...)
	if errors.Is(err, zenity.ErrCanceled) {
		return DirectorySelection{Canceled: true}, nil
	}
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("open native directory picker: %w", err)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return DirectorySelection{Canceled: true}, nil
	}
	abs, err := filepath.Abs(selected)
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("resolve selected directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("inspect selected directory: %w", err)
	}
	if !info.IsDir() {
		return DirectorySelection{}, fmt.Errorf("selected path is not a directory: %s", abs)
	}
	if canonical, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = canonical
	}
	return DirectorySelection{Path: filepath.Clean(abs)}, nil
}

func nearestExistingDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	current := filepath.Clean(path)
	for {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}
