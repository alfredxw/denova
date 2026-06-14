package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxBookRecords = 20

// BookRecord 表示 Nova 数据目录中的一个书籍工作目录。
type BookRecord struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Author       string `json:"author"`
	LastOpenedAt string `json:"last_opened_at"`
}

type bookRegistryData struct {
	Current string       `json:"current"`
	Books   []BookRecord `json:"books"`
	Order   []string     `json:"order,omitempty"`
	Hidden  []string     `json:"hidden,omitempty"`
}

// BookRegistry 持久化当前书籍，并从 Nova 数据目录发现实际存在的书籍工作目录。
type BookRegistry struct {
	path       string
	legacyPath string
	novaDir    string
}

// NewBookRegistry 创建书籍记录管理器。
func NewBookRegistry(novaDir string) *BookRegistry {
	return &BookRegistry{
		path:       filepath.Join(novaDir, "books.json"),
		legacyPath: legacyBookRegistryPath(),
		novaDir:    novaDir,
	}
}

// Current 返回上次打开且仍存在的工作目录。
func (r *BookRegistry) Current() string {
	data := r.load()
	if data.Current == "" {
		return ""
	}
	current, err := filepath.Abs(data.Current)
	if err != nil {
		return ""
	}
	if pathSet(data.Hidden)[current] {
		return ""
	}
	if info, err := os.Stat(current); err == nil && info.IsDir() {
		return current
	}
	return ""
}

// List 返回当前 Nova 数据目录下实际存在的书籍列表。
func (r *BookRegistry) List() []BookRecord {
	data := r.load()
	if strings.TrimSpace(r.novaDir) == "" {
		return sortedRegistryBooks(data)
	}

	books, err := r.scanNovaBooks(data)
	if err == nil {
		return books
	}
	return sortedRegistryBooks(data)
}

func sortedRegistryBooks(data bookRegistryData) []BookRecord {
	hidden := pathSet(data.Hidden)
	books := make([]BookRecord, 0, len(data.Books))
	for _, book := range data.Books {
		if book.Path == "" {
			continue
		}
		absPath, err := filepath.Abs(book.Path)
		if err != nil || hidden[absPath] {
			continue
		}
		book.Path = absPath
		books = append(books, book)
	}
	if len(data.Order) > 0 {
		sortBooksByOrder(books, data.Order, func(i, j int) bool {
			return books[i].LastOpenedAt > books[j].LastOpenedAt
		})
		return books
	}
	sort.SliceStable(books, func(i, j int) bool {
		return books[i].LastOpenedAt > books[j].LastOpenedAt
	})
	return books
}

func (r *BookRegistry) scanNovaBooks(data bookRegistryData) ([]BookRecord, error) {
	absNovaDir, err := filepath.Abs(r.novaDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absNovaDir)
	if err != nil {
		return nil, err
	}

	openedAt := make(map[string]string, len(data.Books))
	for _, book := range data.Books {
		if book.Path == "" {
			continue
		}
		absPath, err := filepath.Abs(book.Path)
		if err != nil {
			continue
		}
		openedAt[absPath] = book.LastOpenedAt
	}
	hidden := pathSet(data.Hidden)

	books := make([]BookRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || isNovaUserDataDir(entry.Name()) {
			continue
		}
		bookPath := filepath.Join(absNovaDir, entry.Name())
		if !isBookWorkspace(bookPath) {
			continue
		}
		if hidden[bookPath] {
			continue
		}
		books = append(books, BookRecord{
			Name:         entry.Name(),
			Path:         bookPath,
			LastOpenedAt: openedAt[bookPath],
		})
	}

	sortBooksByOrder(books, data.Order, func(i, j int) bool {
		return strings.ToLower(books[i].Name) < strings.ToLower(books[j].Name)
	})
	return books, nil
}

func sortBooksByOrder(books []BookRecord, order []string, fallback func(i, j int) bool) {
	rank := make(map[string]int, len(order))
	for i, path := range order {
		if absPath, err := filepath.Abs(path); err == nil {
			if _, exists := rank[absPath]; !exists {
				rank[absPath] = i
			}
		}
	}
	sort.SliceStable(books, func(i, j int) bool {
		leftRank, leftOrdered := rank[books[i].Path]
		rightRank, rightOrdered := rank[books[j].Path]
		if leftOrdered && rightOrdered {
			return leftRank < rightRank
		}
		if leftOrdered != rightOrdered {
			return leftOrdered
		}
		return fallback(i, j)
	})
}

func pathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		set[absPath] = true
	}
	return set
}

func isNovaUserDataDir(name string) bool {
	switch name {
	case "book_meta", "styles":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func isBookWorkspace(path string) bool {
	markers := []string{
		filepath.Join(path, ".nova"),
		filepath.Join(path, "book.json"),
		filepath.Join(path, "ideas.md"),
		filepath.Join(path, "brainstorm.md"),
		filepath.Join(path, "chapters"),
		filepath.Join(path, "setting"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// Touch 记录并置顶一个书籍工作目录。
func (r *BookRegistry) Touch(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("路径不是目录")
	}

	data := r.load()
	now := time.Now().Format(time.RFC3339)
	record := BookRecord{
		Name:         filepath.Base(absPath),
		Path:         absPath,
		LastOpenedAt: now,
	}
	data.Hidden = removePath(data.Hidden, absPath)
	if len(data.Order) > 0 {
		found := false
		for i, book := range data.Books {
			bookPath, err := filepath.Abs(book.Path)
			if err == nil && bookPath == absPath {
				data.Books[i] = record
				found = true
				break
			}
		}
		if !found {
			data.Books = append(data.Books, record)
		}
		if !pathSet(data.Order)[absPath] {
			data.Order = append(data.Order, absPath)
		}
	} else {
		books := []BookRecord{record}
		for _, book := range data.Books {
			bookPath, err := filepath.Abs(book.Path)
			if book.Path == "" || (err == nil && bookPath == absPath) {
				continue
			}
			books = append(books, book)
			if len(books) >= maxBookRecords {
				break
			}
		}
		data.Books = books
	}
	data.Current = absPath
	return r.save(data)
}

// Remove 从书架隐藏一个书籍记录，不删除磁盘文件。
func (r *BookRegistry) Remove(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	data := r.load()
	data.Hidden = appendUniquePath(data.Hidden, absPath)
	books := make([]BookRecord, 0, len(data.Books))
	for _, book := range data.Books {
		bookPath, err := filepath.Abs(book.Path)
		if err == nil && bookPath == absPath {
			continue
		}
		books = append(books, book)
	}
	data.Order = removePath(data.Order, absPath)
	current, _ := filepath.Abs(data.Current)
	if current == absPath {
		data.Current = ""
		if len(books) > 0 {
			data.Current = books[0].Path
		}
	}
	data.Books = books
	return r.save(data)
}

// Reorder 保存书籍管理页的自定义排序。
func (r *BookRegistry) Reorder(paths []string) error {
	data := r.load()
	seen := make(map[string]bool, len(paths))
	order := make([]string, 0, len(paths))
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil || seen[absPath] {
			continue
		}
		seen[absPath] = true
		order = append(order, absPath)
	}
	for _, book := range r.List() {
		if !seen[book.Path] {
			order = append(order, book.Path)
		}
	}
	data.Order = order
	return r.save(data)
}

func appendUniquePath(paths []string, path string) []string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return paths
	}
	for _, item := range paths {
		itemAbs, err := filepath.Abs(item)
		if err == nil && itemAbs == absPath {
			return paths
		}
	}
	return append(paths, absPath)
}

func removePath(paths []string, path string) []string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return paths
	}
	next := make([]string, 0, len(paths))
	for _, item := range paths {
		itemAbs, err := filepath.Abs(item)
		if err != nil || itemAbs == absPath {
			continue
		}
		next = append(next, itemAbs)
	}
	return next
}

func (r *BookRegistry) load() bookRegistryData {
	var data bookRegistryData
	raw, err := os.ReadFile(r.path)
	if err != nil && r.legacyPath != "" && r.legacyPath != r.path {
		raw, err = os.ReadFile(r.legacyPath)
	}
	if err != nil {
		return data
	}
	_ = json.Unmarshal(raw, &data)
	return data
}

func (r *BookRegistry) save(data bookRegistryData) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, raw, 0o644)
}

func legacyBookRegistryPath() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "nova", "books.json")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".nova", "books.json")
	}
	return filepath.Join(".", ".nova-books.json")
}
