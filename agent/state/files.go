package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const privateDirectory = ".git"

func scanFiles(root string) ([]File, error) {
	files := make([]File, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		if relative == privateDirectory || strings.HasPrefix(relative, privateDirectory+"/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic links are not allowed: %s", ErrInvalidPath, relative)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: non-regular file is not allowed: %s", ErrInvalidPath, relative)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, File{Path: relative, Content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Agent state: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func snapshotFromFiles(files []File) Snapshot {
	files = cloneFiles(files)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	digest := sha256.New()
	for _, file := range files {
		io.WriteString(digest, file.Path)
		digest.Write([]byte{0})
		encodedLength, _ := json.Marshal(len(file.Content))
		digest.Write(encodedLength)
		digest.Write([]byte{0})
		digest.Write(file.Content)
		digest.Write([]byte{0})
	}
	return Snapshot{Revision: hex.EncodeToString(digest.Sum(nil)), files: files}
}

func normalizePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", ErrInvalidPath
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned == privateDirectory || strings.HasPrefix(cleaned, privateDirectory+"/") {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

func applyChanges(base Snapshot, changes []Change) (Snapshot, error) {
	files := make(map[string][]byte, len(base.files)+len(changes))
	for _, file := range base.files {
		files[file.Path] = append([]byte(nil), file.Content...)
	}
	seen := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		path, err := normalizePath(change.Path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: %q", err, change.Path)
		}
		if _, duplicate := seen[path]; duplicate {
			return Snapshot{}, fmt.Errorf("duplicate Agent state change path %q", path)
		}
		seen[path] = struct{}{}
		if change.Delete {
			delete(files, path)
			continue
		}
		files[path] = append([]byte(nil), change.Content...)
	}
	result := make([]File, 0, len(files))
	for path, content := range files {
		result = append(result, File{Path: path, Content: content})
	}
	return snapshotFromFiles(result), nil
}

func writeSnapshot(root string, snapshot Snapshot) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == privateDirectory {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	for _, file := range snapshot.files {
		path, err := normalizePath(file.Path)
		if err != nil {
			return err
		}
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(absolute), ".denova-state-*")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		if err := temporary.Chmod(0o600); err == nil {
			_, err = temporary.Write(file.Content)
		}
		if err == nil {
			err = temporary.Sync()
		}
		if closeErr := temporary.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(temporaryPath, absolute)
		}
		if err != nil {
			os.Remove(temporaryPath)
			return err
		}
	}
	return syncTreeDirectories(root)
}

func copySnapshot(root string, snapshot Snapshot) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	for _, file := range snapshot.files {
		path, err := normalizePath(file.Path)
		if err != nil {
			return err
		}
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			return err
		}
		output, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err = output.Write(file.Content); err == nil {
			err = output.Sync()
		}
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	return syncTreeDirectories(root)
}

func atomicJSON(path string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".denova-state-json-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(encoded)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryPath, path)
	}
	if err == nil {
		err = syncDirectory(filepath.Dir(path))
	}
	if err != nil {
		os.Remove(temporaryPath)
	}
	return err
}

func removeDurable(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncTreeDirectories(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && entry.IsDir() && filepath.Dir(path) == root && entry.Name() == privateDirectory {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func readJSON(path string, value any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func removeEmptyParents(root, start string) error {
	for current := filepath.Dir(start); current != root && strings.HasPrefix(current, root+string(filepath.Separator)); current = filepath.Dir(current) {
		err := os.Remove(current)
		if err == nil {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		break
	}
	return nil
}
