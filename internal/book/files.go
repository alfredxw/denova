package book

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileNode 表示文件树节点。
type FileNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" 或 "dir"
	Children []*FileNode `json:"children,omitempty"`
}

// Service 提供作品工作区文件管理能力。
type Service struct {
	workspace string
	styleRoot string
}

// NewService 创建作品文件服务。
func NewService(workspace string) *Service {
	return &Service{workspace: workspace, styleRoot: filepath.Join(workspace, "setting", "styles")}
}

// NewServiceWithStyleRoot 创建作品文件服务，并指定用户级风格参考目录。
func NewServiceWithStyleRoot(workspace, styleRoot string) *Service {
	if strings.TrimSpace(styleRoot) == "" {
		styleRoot = filepath.Join(workspace, "setting", "styles")
	}
	return &Service{workspace: workspace, styleRoot: styleRoot}
}

// UserStyleDir 返回用户级风格参考目录。
func UserStyleDir(novaDir string) string {
	return filepath.Join(novaDir, "styles")
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
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("路径是目录而非文件")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// StyleFiles 返回用户级 styles/ 下所有可用的风格参考文件。
func (s *Service) StyleFiles() ([]string, error) {
	root := s.styleRoot
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if name != "." && strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !isStyleReferenceFile(name) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// ReadStyleFile 安全读取用户级 styles/ 下指定的风格参考文件。
func (s *Service) ReadStyleFile(stylePath string) (string, error) {
	if strings.TrimSpace(stylePath) == "" {
		return "", errors.New("风格参考路径不能为空")
	}
	if filepath.IsAbs(stylePath) {
		return "", errors.New("不允许使用绝对路径")
	}
	if !isStyleReferenceFile(stylePath) {
		return "", errors.New("风格参考只支持 Markdown 或 TXT 文件")
	}
	absPath, err := safeStyleReferencePath(s.styleRoot, stylePath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isStyleReferenceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}

func safeStyleReferencePath(root, stylePath string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("风格参考目录未配置")
	}
	clean := filepath.Clean(filepath.FromSlash(stylePath))
	if clean == "." || clean == ".." || strings.HasPrefix(filepath.ToSlash(clean), "../") {
		return "", errors.New("风格参考路径不在用户 styles 目录范围内")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(absRoot, clean)
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", errors.New("风格参考路径不在用户 styles 目录范围内")
	}
	return absPath, nil
}

// WriteFile 写入 workspace 内文件内容，必要时创建父目录。
func (s *Service) WriteFile(relPath, content string) error {
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
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
		return os.MkdirAll(absPath, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
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
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(absPath)
	}
	return os.Remove(absPath)
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

	return filepath.ToSlash(filepath.Join(filepath.Dir(relPath), newName)), nil
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
	return CopyPath(from, to)
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
	return os.Rename(from, to)
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
		return compareFileNodeNames(nodes[i].Name, nodes[j].Name) < 0
	})
	return nodes, nil
}

func compareFileNodeNames(left, right string) int {
	if cmp := compareChapterLikeNames(left, right); cmp != 0 {
		return cmp
	}
	return strings.Compare(left, right)
}
