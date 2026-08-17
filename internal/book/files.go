package book

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrFileRevisionConflict 表示保存时文件已被其他来源更新，调用方应重新读取后再写入。
var ErrFileRevisionConflict = errors.New("文件已被其他来源更新，请重新加载后再保存")

// FileNode 表示文件树节点。
type FileNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" 或 "dir"
	Children []*FileNode `json:"children,omitempty"`
}

// Service 提供作品工作区文件管理能力。
type Service struct {
	workspace           string
	summaryMu           sync.Mutex
	chapters            map[string]cachedChapterSummary
	invalidatedChapters map[string]struct{}
}

// NewService 创建作品文件服务。
func NewService(workspace string) *Service {
	return &Service{
		workspace:           workspace,
		chapters:            make(map[string]cachedChapterSummary),
		invalidatedChapters: make(map[string]struct{}),
	}
}

// Workspace 返回当前作品工作目录。
func (s *Service) Workspace() string {
	return s.workspace
}

// Tree 递归扫描 workspace 目录返回文件树。
func (s *Service) Tree() ([]*FileNode, error) {
	return BuildFileTree(s.workspace)
}

// ReadFile 读取 workspace 内文件内容。
func (s *Service) ReadFile(relPath string) (string, error) {
	content, _, err := s.ReadFileWithRevision(relPath)
	return content, err
}

// ReadFileWithRevision reads one stable byte snapshot and derives its revision
// from those exact bytes, avoiding a separate stat/read race.
func (s *Service) ReadFileWithRevision(relPath string) (string, string, error) {
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return "", "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return "", "", errors.New("路径是目录而非文件")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", "", err
	}
	return string(data), contentRevision(data), nil
}

// FileRevision returns a content-addressed revision used to reject stale writes.
func (s *Service) FileRevision(relPath string) (string, error) {
	_, revision, err := s.ReadFileWithRevision(relPath)
	return revision, err
}

// WriteFile 写入 workspace 内文件内容，必要时创建父目录。
func (s *Service) WriteFile(relPath, content string) error {
	_, err := s.WriteFileIfRevision(relPath, content, "")
	return err
}

// WriteBinaryFile writes binary content inside the workspace using the same
// path boundary as text file writes.
func (s *Service) WriteBinaryFile(relPath string, data []byte) error {
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return err
	}
	s.InvalidateSummary([]string{relPath}, false)
	return nil
}

// WriteFileIfRevision 写入文件；expectedRevision 非空时，只有文件仍处于该版本才允许写入。
func (s *Service) WriteFileIfRevision(relPath, content, expectedRevision string) (string, error) {
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", err
	}
	if expectedRevision != "" {
		data, err := os.ReadFile(absPath)
		if err != nil {
			return "", err
		}
		if contentRevision(data) != expectedRevision {
			return "", ErrFileRevisionConflict
		}
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	s.InvalidateSummary([]string{relPath}, false)
	return contentRevision([]byte(content)), nil
}

func contentRevision(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// Create 新建 workspace 内文件或目录。
func (s *Service) Create(relPath, itemType, content string) error {
	if itemType != "file" && itemType != "dir" {
		return errors.New("type 只能是 file 或 dir")
	}

	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if itemType == "dir" {
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			return err
		}
		s.InvalidateSummary([]string{relPath}, false)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return err
	}
	s.InvalidateSummary([]string{relPath}, false)
	return nil
}

// Delete 直接删除 workspace 内文件或目录；恢复依赖 Nova 版本历史。
func (s *Service) Delete(relPath string) error {
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return err
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return err
	}
	// Lstat keeps a symbolic-link leaf as the deletion target instead of
	// following it. RemoveAll likewise removes the link, not its destination.
	info, err := os.Lstat(absPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.RemoveAll(absPath); err != nil {
			return err
		}
		s.InvalidateSummary([]string{relPath}, false)
		return nil
	}
	if err := os.Remove(absPath); err != nil {
		return err
	}
	s.InvalidateSummary([]string{relPath}, false)
	return nil
}

// Rename 重命名同目录下的文件或目录，并返回新相对路径。
func (s *Service) Rename(relPath, newName string) (string, error) {
	if err := ValidateNewName(newName); err != nil {
		return "", err
	}

	from, err := SafePath(s.workspace, relPath)
	if err != nil {
		return "", err
	}
	to := filepath.Join(filepath.Dir(from), newName)
	if _, err := os.Stat(to); err == nil {
		return "", os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(from, to); err != nil {
		return "", err
	}
	toRel := filepath.ToSlash(filepath.Join(filepath.Dir(relPath), newName))
	s.InvalidateSummary([]string{relPath, toRel}, false)
	return toRel, nil
}

// Copy 复制 workspace 内文件或目录。
func (s *Service) Copy(fromRel, toRel string) error {
	from, err := SafePath(s.workspace, fromRel)
	if err != nil {
		return err
	}
	to, err := SafePath(s.workspace, toRel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := CopyPath(from, to); err != nil {
		return err
	}
	s.InvalidateSummary([]string{toRel}, false)
	return nil
}

// Move 移动 workspace 内文件或目录。
func (s *Service) Move(fromRel, toRel string) error {
	from, err := SafePath(s.workspace, fromRel)
	if err != nil {
		return err
	}
	to, err := SafePath(s.workspace, toRel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}
	s.InvalidateSummary([]string{fromRel, toRel}, false)
	return nil
}

// InvalidateSummary marks chapter projections changed by external editors,
// tools, or another Service instance. The next Summary call re-reads only the
// affected chapter content; resync discards the complete rebuildable cache.
func (s *Service) InvalidateSummary(paths []string, resync bool) {
	if s == nil {
		return
	}
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	if resync {
		s.chapters = make(map[string]cachedChapterSummary)
		s.invalidatedChapters = make(map[string]struct{})
		return
	}
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
		if path == "chapters" || strings.HasPrefix(path, "chapters/") {
			s.invalidatedChapters[path] = struct{}{}
		}
	}
}

// SafePath 将相对路径解析为 workspace 内的绝对路径，并禁止访问隐藏目录。
func SafePath(workspace, relPath string) (string, error) {
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

// BuildFileTree 递归构建文件树，跳过隐藏文件和隐藏目录。
func BuildFileTree(dir string) ([]*FileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []*FileNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		node := &FileNode{Name: name}
		if entry.IsDir() {
			node.Type = "dir"
			children, err := BuildFileTree(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			node.Children = children
		} else {
			node.Type = "file"
		}
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type == "dir"
		}
		return CompareFileNodeNames(nodes[i].Name, nodes[j].Name) < 0
	})
	return nodes, nil
}

// CompareFileNodeNames defines the canonical project tree order, including
// natural chapter and volume ordinals used by Writing projects.
func CompareFileNodeNames(left, right string) int {
	if cmp := compareChapterLikeNames(left, right); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(strings.ToLower(left), strings.ToLower(right)); cmp != 0 {
		return cmp
	}
	return strings.Compare(left, right)
}
