package interactive

import (
	"os"
	"path/filepath"
)

// listDirectorModuleFiles owns the common collection behavior while each
// module keeps its type-specific decoding, ownership, and ordering rules.
func listDirectorModuleFiles[T any](
	dir string,
	ensureBuiltins func() error,
	parse func(string) (T, error),
	invalid func(id, path string, err error) T,
	applyOwnership func(T) T,
	sortItems func([]T),
) ([]T, error) {
	if err := ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]T, 0, len(files))
	for _, file := range files {
		item, err := parse(file)
		if err != nil {
			items = append(items, invalid(moduleIDFromPath(file), file, err))
			continue
		}
		items = append(items, applyOwnership(item))
	}
	sortItems(items)
	return items, nil
}

func getDirectorModuleFile[T any](
	dir, id string,
	ensureBuiltins func() error,
	parse func(string) (T, error),
	applyOwnership func(T) T,
) (T, error) {
	var zero T
	if err := ensureBuiltins(); err != nil {
		return zero, err
	}
	item, err := parse(filepath.Join(dir, id+".json"))
	if err != nil {
		return zero, err
	}
	return applyOwnership(item), nil
}

// ensureBuiltinModuleFiles restores only files whose type-specific policy says
// they no longer represent a current built-in module.
func ensureBuiltinModuleFiles[T any](
	dir string,
	builtins []T,
	id func(T) string,
	parse func(string) (T, error),
	keep func(current, builtin T) bool,
	write func(string, T) error,
) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, builtin := range builtins {
		path := filepath.Join(dir, id(builtin)+".json")
		if current, err := parse(path); err == nil && keep(current, builtin) {
			continue
		}
		if err := write(path, builtin); err != nil {
			return err
		}
	}
	return nil
}

func moduleIDFromPath(path string) string {
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))])
}
