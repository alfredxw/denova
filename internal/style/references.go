package style

import (
	"context"
	"denova/internal/revisionfile"
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
		_, err = revisionfile.ReplaceIfRevision(context.Background(), path, "", data, revisionfile.Options{FileMode: 0644, DirectoryMode: 0755})
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
	_, err := revisionfile.ReplaceIfRevision(context.Background(), filepath.Join(dir, filename), revisionfile.MissingRevision, data, revisionfile.Options{FileMode: 0644, DirectoryMode: 0755})
	return err
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
	snapshot, err := revisionfile.Read(context.Background(), abs)
	if err != nil {
		return FileDocument{}, err
	}
	if !snapshot.Exists {
		return FileDocument{}, os.ErrNotExist
	}
	ref, err := l.referenceFromFile(abs)
	if err != nil {
		return FileDocument{}, err
	}
	name, description := summarizeMarkdown(filepath.Base(abs), string(snapshot.Content))
	ref.Name, ref.Description, ref.Size = name, description, int64(len(snapshot.Content))
	return FileDocument{Reference: ref, Content: string(snapshot.Content), Revision: snapshot.Revision}, nil
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
	_, err := revisionfile.Mutate(context.Background(), abs, revisionfile.Options{}, func(current revisionfile.Snapshot) ([]byte, error) {
		if !current.Exists {
			return nil, os.ErrNotExist
		}
		if req.BaseRevision != "" && req.BaseRevision != current.Revision {
			return nil, ErrReferenceRevisionConflict
		}
		return []byte(ensureTrailingNewline(content)), nil
	})
	if err != nil {
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
	abs := l.AbsPath(stored)
	return revisionfile.WithFiles(context.Background(), []string{abs}, func(files *revisionfile.LockedFiles) error { return files.Replace(abs, nil, false) })
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
	snapshot, err := revisionfile.Read(context.Background(), path)
	if err != nil {
		return Reference{}, err
	}
	if !snapshot.Exists {
		return Reference{}, os.ErrNotExist
	}
	name, desc := summarizeMarkdown(filepath.Base(path), string(snapshot.Content))
	return Reference{
		Name:        name,
		Description: desc,
		Path:        path,
		DisplayPath: StoragePath(filepath.Base(path)),
		Size:        info.Size(),
		UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}
