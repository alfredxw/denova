package versions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Service) collectVisibleFiles() ([]versionFileData, error) {
	return collectVersionFiles(s.workspace, s.workspace)
}

func (s *Service) collectSnapshotFiles(id string) (map[string]string, error) {
	files, err := collectVersionFiles(s.snapshotDir(id), s.snapshotDir(id))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(files))
	for _, file := range files {
		if file.Path == "manifest.json" {
			continue
		}
		result[file.Path] = file.Hash
	}
	return result, nil
}

func collectVersionFiles(root, base string) ([]versionFileData, error) {
	files := []versionFileData{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hashBytes := sha256.Sum256(data)
		text := isTextBytes(data)
		chars := 0
		if text {
			chars = utf8.RuneCount(data)
		}
		files = append(files, versionFileData{
			Path:  filepath.ToSlash(rel),
			Abs:   path,
			Hash:  hex.EncodeToString(hashBytes[:]),
			Size:  info.Size(),
			Chars: chars,
			Text:  text,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func isTextBytes(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func randomVersionSuffix(files []versionFileData) string {
	h := sha256.New()
	_, _ = io.WriteString(h, time.Now().Format(time.RFC3339Nano))
	for _, file := range files {
		_, _ = io.WriteString(h, file.Path)
		_, _ = io.WriteString(h, file.Hash)
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:])[:8]
}

func copyVersionFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func safeVisiblePath(workspace, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("路径不能为空")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.New("不允许使用绝对路径")
	}

	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("路径不在 workspace 范围内")
	}

	for _, part := range strings.Split(cleanRel, string(filepath.Separator)) {
		if part == "" || strings.HasPrefix(part, ".") {
			return "", errors.New("不允许操作隐藏文件或隐藏目录")
		}
	}

	cleanWorkspace := filepath.Clean(workspace)
	absPath := filepath.Clean(filepath.Join(cleanWorkspace, cleanRel))
	if absPath != cleanWorkspace && !strings.HasPrefix(absPath, cleanWorkspace+string(filepath.Separator)) {
		return "", errors.New("路径不在 workspace 范围内")
	}
	return absPath, nil
}
