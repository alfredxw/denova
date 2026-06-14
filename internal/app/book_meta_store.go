package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nova/internal/book"
)

// StoredBookMeta 表示保存在用户数据目录中的书籍元信息。
type StoredBookMeta struct {
	Path string `json:"path"`
	book.BookMeta
}

// BookMetaStore 管理用户级书籍元信息，不写入书籍 workspace。
type BookMetaStore struct {
	dir string
}

// NewBookMetaStore 创建书籍元信息存储。
func NewBookMetaStore(novaDir string) *BookMetaStore {
	return &BookMetaStore{dir: filepath.Join(novaDir, "book_meta")}
}

// Read 读取书籍元信息，优先使用用户数据目录，兼容旧 workspace/book.json。
func (s *BookMetaStore) Read(path string) (book.BookMeta, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}

	data, err := os.ReadFile(s.metaPath(absPath))
	if err == nil {
		var stored StoredBookMeta
		if err := json.Unmarshal(data, &stored); err != nil {
			return book.BookMeta{}, fmt.Errorf("解析书籍元信息失败: %w", err)
		}
		return stored.BookMeta, nil
	}
	if !os.IsNotExist(err) {
		return book.BookMeta{}, fmt.Errorf("读取书籍元信息失败: %w", err)
	}

	return book.ReadBookMetaFromDir(absPath), nil
}

// Write 写入书籍元信息到用户数据目录，并维护创建/更新时间。
func (s *BookMetaStore) Write(path string, meta book.BookMeta) (book.BookMeta, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		if existing, err := s.Read(absPath); err == nil && existing.CreatedAt != "" {
			meta.CreatedAt = existing.CreatedAt
		} else {
			meta.CreatedAt = now
		}
	}
	meta.UpdatedAt = now

	if meta.Title == "" {
		meta.Title = filepath.Base(absPath)
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return book.BookMeta{}, fmt.Errorf("创建书籍元信息目录失败: %w", err)
	}

	stored := StoredBookMeta{Path: absPath, BookMeta: meta}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return book.BookMeta{}, fmt.Errorf("序列化书籍元信息失败: %w", err)
	}
	if err := os.WriteFile(s.metaPath(absPath), data, 0o644); err != nil {
		return book.BookMeta{}, fmt.Errorf("写入书籍元信息失败: %w", err)
	}
	return meta, nil
}

func (s *BookMetaStore) metaPath(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}
