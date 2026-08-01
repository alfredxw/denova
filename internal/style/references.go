package style

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrReferenceRevisionConflict 表示文风参考文件在编辑器读取后已被外部更新。
var ErrReferenceRevisionConflict = errors.New("文风参考文件已被其他来源更新，请重新加载后再保存")

const (
	DirName            = "styles"
	DisplayDir         = ".denova/styles"
	MaxContentBytes    = 160 * 1024
	MaxDescriptionSize = 240
)

type Library struct {
	novaDir string
}

type Reference struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	DisplayPath string `json:"display_path"`
	Size        int64  `json:"size,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Missing     bool   `json:"missing,omitempty"`
	Error       string `json:"error,omitempty"`
}

type WriteRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Filename    string `json:"filename,omitempty"`
	Content     string `json:"content"`
}

type FileDocument struct {
	Reference Reference `json:"reference"`
	Content   string    `json:"content"`
	Revision  string    `json:"revision"`
}

type UpdateRequest struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	BaseRevision string `json:"base_revision"`
}

func NewLibrary(novaDir string) *Library {
	return &Library{novaDir: strings.TrimSpace(novaDir)}
}

func (l *Library) List() ([]Reference, error) {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return nil, fmt.Errorf("nova_dir 不可用，无法读取文风参考")
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(l.dir())
	if err != nil {
		return nil, err
	}
	refs := make([]Reference, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isStyleFile(entry.Name()) {
			continue
		}
		ref, err := l.referenceFromFile(filepath.Join(l.dir(), entry.Name()))
		if err != nil {
			refs = append(refs, Reference{
				Name:        strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
				Path:        filepath.Join(l.dir(), entry.Name()),
				DisplayPath: StoragePath(entry.Name()),
				Missing:     true,
				Error:       err.Error(),
			})
			continue
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].DisplayPath < refs[j].DisplayPath
	})
	return refs, nil
}

func (l *Library) Resolve(paths []string) []Reference {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return nil
	}
	refs := make([]Reference, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		stored := NormalizeStoragePath(path)
		if stored == "" || seen[stored] {
			continue
		}
		seen[stored] = true
		abs := l.AbsPath(stored)
		ref, err := l.referenceFromFile(abs)
		if err != nil {
			refs = append(refs, Reference{
				Name:        strings.TrimSuffix(filepath.Base(stored), filepath.Ext(stored)),
				Path:        abs,
				DisplayPath: stored,
				Missing:     true,
				Error:       err.Error(),
			})
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

func (l *Library) Write(req WriteRequest) (Reference, error) {
	return l.write(req, false)
}

// Create writes a new style reference and fails if the normalized target
// already exists. Write retains the UI's intentional replace semantics.
func (l *Library) Create(req WriteRequest) (Reference, error) {
	return l.write(req, true)
}

func (l *Library) write(req WriteRequest, exclusive bool) (Reference, error) {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return Reference{}, fmt.Errorf("nova_dir 不可用，无法写入文风参考")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Reference{}, fmt.Errorf("文风参考内容不能为空")
	}
	description := oneLine(req.Description)
	if utf8.RuneCountInString(description) > MaxDescriptionSize {
		return Reference{}, fmt.Errorf("description 超过 %d 个字符 / exceeds %d characters", MaxDescriptionSize, MaxDescriptionSize)
	}
	content = ensureReferenceHeader(content, req.Name, req.Description)
	if len([]byte(content)) > MaxContentBytes {
		return Reference{}, fmt.Errorf("content 超过 %d 字节 / exceeds %d bytes", MaxContentBytes, MaxContentBytes)
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return Reference{}, err
	}
	filename := filenameForWrite(req.Filename, req.Name)
	path := filepath.Join(l.dir(), filename)
	data := []byte(ensureTrailingNewline(content))
	var err error
	if exclusive {
		err = createReferenceFile(l.dir(), filename, data)
	} else {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		return Reference{}, err
	}
	ref, err := l.referenceFromFile(path)
	if err != nil {
		return Reference{}, err
	}
	if strings.TrimSpace(req.Name) != "" {
		ref.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Description) != "" {
		ref.Description = truncateRunes(strings.TrimSpace(req.Description), MaxDescriptionSize)
	}
	return ref, nil
}

func createReferenceFile(dir, filename string, data []byte) error {
	temp, err := os.CreateTemp(dir, "."+filename+".denova-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempPath, filepath.Join(dir, filename)); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("文风参考已存在: %s", StoragePath(filename))
		}
		return err
	}
	return nil
}

func (l *Library) Read(path string) (FileDocument, error) {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return FileDocument{}, fmt.Errorf("nova_dir 不可用，无法读取文风参考")
	}
	stored := NormalizeStoragePath(path)
	if stored == "" {
		return FileDocument{}, fmt.Errorf("文风参考路径不能为空")
	}
	abs := l.AbsPath(stored)
	info, err := os.Stat(abs)
	if err != nil {
		return FileDocument{}, err
	}
	if info.IsDir() {
		return FileDocument{}, fmt.Errorf("文风参考路径是目录")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return FileDocument{}, err
	}
	ref, err := l.referenceFromFile(abs)
	if err != nil {
		return FileDocument{}, err
	}
	return FileDocument{
		Reference: ref,
		Content:   string(data),
		Revision:  fileRevision(info),
	}, nil
}

func (l *Library) Update(req UpdateRequest) (FileDocument, error) {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return FileDocument{}, fmt.Errorf("nova_dir 不可用，无法写入文风参考")
	}
	stored := NormalizeStoragePath(req.Path)
	if stored == "" {
		return FileDocument{}, fmt.Errorf("文风参考路径不能为空")
	}
	content := req.Content
	if strings.TrimSpace(content) == "" {
		return FileDocument{}, fmt.Errorf("文风参考内容不能为空")
	}
	if len([]byte(content)) > MaxContentBytes {
		return FileDocument{}, fmt.Errorf("content 超过 %d 字节 / exceeds %d bytes", MaxContentBytes, MaxContentBytes)
	}
	abs := l.AbsPath(stored)
	info, err := os.Stat(abs)
	if err != nil {
		return FileDocument{}, err
	}
	if info.IsDir() {
		return FileDocument{}, fmt.Errorf("文风参考路径是目录")
	}
	if req.BaseRevision != "" && fileRevision(info) != req.BaseRevision {
		return FileDocument{}, ErrReferenceRevisionConflict
	}
	if err := os.WriteFile(abs, []byte(ensureTrailingNewline(content)), 0o644); err != nil {
		return FileDocument{}, err
	}
	return l.Read(stored)
}

func (l *Library) Delete(path string) error {
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return fmt.Errorf("nova_dir 不可用，无法删除文风参考")
	}
	stored := NormalizeStoragePath(path)
	if stored == "" {
		return fmt.Errorf("文风参考路径不能为空")
	}
	return os.Remove(l.AbsPath(stored))
}

func (l *Library) AbsPath(path string) string {
	stored := NormalizeStoragePath(path)
	if stored == "" || l == nil {
		return ""
	}
	return filepath.Join(l.dir(), filepath.Base(stored))
}

func (l *Library) dir() string {
	return filepath.Join(l.novaDir, DirName)
}

func (l *Library) referenceFromFile(path string) (Reference, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Reference{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Reference{}, err
	}
	name, desc := summarizeMarkdown(filepath.Base(path), string(data))
	return Reference{
		Name:        name,
		Description: desc,
		Path:        path,
		DisplayPath: StoragePath(filepath.Base(path)),
		Size:        info.Size(),
		UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}
