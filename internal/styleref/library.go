package styleref

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

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
	if l == nil || strings.TrimSpace(l.novaDir) == "" {
		return Reference{}, fmt.Errorf("nova_dir 不可用，无法写入文风参考")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Reference{}, fmt.Errorf("文风参考内容不能为空")
	}
	content = ensureReferenceHeader(content, req.Name, req.Description)
	content = trimBytes(content, MaxContentBytes)
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return Reference{}, err
	}
	filename := filenameForWrite(req.Filename, req.Name)
	path := filepath.Join(l.dir(), filename)
	if err := os.WriteFile(path, []byte(ensureTrailingNewline(content)), 0o644); err != nil {
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

func NormalizeStoragePath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if !isStyleFile(base) {
		base += ".md"
	}
	base = sanitizeFilename(base)
	if base == "" {
		return ""
	}
	return StoragePath(base)
}

func StoragePath(filename string) string {
	filename = sanitizeFilename(filepath.Base(strings.TrimSpace(filename)))
	if filename == "" {
		return ""
	}
	if !isStyleFile(filename) {
		filename += ".md"
	}
	return filepath.ToSlash(filepath.Join(DisplayDir, filename))
}

func isStyleFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func filenameForWrite(filename, name string) string {
	filename = sanitizeFilename(filepath.Base(strings.TrimSpace(filename)))
	if filename == "" {
		filename = slugFilename(name)
	}
	if filename == "" {
		filename = fmt.Sprintf("style-%d.md", time.Now().UnixNano())
	}
	if !isStyleFile(filename) {
		filename += ".md"
	}
	if strings.ToLower(filepath.Ext(filename)) != ".md" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".md"
	}
	return filename
}

func sanitizeFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(filename)
	filename = strings.Trim(filename, ". ")
	if filename == "" || filename == "." || filename == ".." {
		return ""
	}
	var out strings.Builder
	for _, r := range filename {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			out.WriteRune(r)
		case r == '.', r == '-', r == '_':
			out.WriteRune(r)
		case unicode.IsSpace(r):
			out.WriteByte('-')
		}
	}
	cleaned := strings.Trim(out.String(), ".- _")
	if cleaned == "" {
		return ""
	}
	return cleaned
}

func slugFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var out strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			out.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r), r == '-', r == '_':
			if !lastDash && out.Len() > 0 {
				out.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(out.String(), "-") + ".md"
}

func summarizeMarkdown(filename, content string) (string, string) {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	desc := ""
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" || trimmed == "---" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") && name == strings.TrimSuffix(filename, filepath.Ext(filename)) {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		if trimmed != "" {
			desc = truncateRunes(trimmed, MaxDescriptionSize)
			break
		}
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	return strings.TrimSpace(name), desc
}

func ensureTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}

func ensureReferenceHeader(content, name, description string) string {
	content = strings.TrimSpace(content)
	if hasMarkdownH1(content) {
		return content
	}
	name = oneLine(name)
	description = truncateRunes(oneLine(description), MaxDescriptionSize)
	if name == "" && description == "" {
		return content
	}
	var sb strings.Builder
	if name != "" {
		fmt.Fprintf(&sb, "# %s\n\n", name)
	}
	if description != "" {
		fmt.Fprintf(&sb, "> %s\n\n", description)
	}
	sb.WriteString(content)
	return sb.String()
}

func hasMarkdownH1(content string) bool {
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return true
		}
	}
	return false
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func trimBytes(value string, limit int) string {
	if limit <= 0 || len([]byte(value)) <= limit {
		return value
	}
	used := 0
	var out strings.Builder
	for _, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = len(string(r))
		}
		if used+size > limit {
			break
		}
		out.WriteRune(r)
		used += size
	}
	return strings.TrimSpace(out.String())
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
