package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StoredMeta is the user-data representation of Book metadata. Path is kept
// with the payload so the hashed filename never obscures which workspace owns
// a record during inspection or recovery.
type StoredMeta struct {
	Path string `json:"path"`
	BookMeta
}

// MetaStore owns user-level Book metadata without writing into Book content.
// Read retains the legacy workspace book.json fallback for existing projects.
type MetaStore struct {
	dir string
}

func NewMetaStore(denovaDir string) *MetaStore {
	return &MetaStore{dir: filepath.Join(denovaDir, "book_meta")}
}

func (store *MetaStore) Read(path string) (BookMeta, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return BookMeta{}, fmt.Errorf("resolve Book path: %w", err)
	}

	data, err := os.ReadFile(store.metaPath(absolutePath))
	if err == nil {
		var stored StoredMeta
		if err := json.Unmarshal(data, &stored); err != nil {
			return BookMeta{}, fmt.Errorf("decode Book metadata: %w", err)
		}
		return stored.BookMeta, nil
	}
	if !os.IsNotExist(err) {
		return BookMeta{}, fmt.Errorf("read Book metadata: %w", err)
	}
	return ReadBookMetaFromDir(absolutePath), nil
}

func (store *MetaStore) Write(path string, meta BookMeta) (BookMeta, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return BookMeta{}, fmt.Errorf("resolve Book path: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		if existing, readErr := store.Read(absolutePath); readErr == nil && existing.CreatedAt != "" {
			meta.CreatedAt = existing.CreatedAt
		} else {
			meta.CreatedAt = now
		}
	}
	meta.UpdatedAt = now
	if meta.Title == "" {
		meta.Title = filepath.Base(absolutePath)
	}

	if err := os.MkdirAll(store.dir, 0o755); err != nil {
		return BookMeta{}, fmt.Errorf("create Book metadata directory: %w", err)
	}
	data, err := json.MarshalIndent(StoredMeta{Path: absolutePath, BookMeta: meta}, "", "  ")
	if err != nil {
		return BookMeta{}, fmt.Errorf("encode Book metadata: %w", err)
	}
	if err := os.WriteFile(store.metaPath(absolutePath), data, 0o644); err != nil {
		return BookMeta{}, fmt.Errorf("write Book metadata: %w", err)
	}
	return meta, nil
}

func (store *MetaStore) metaPath(absolutePath string) string {
	digest := sha256.Sum256([]byte(absolutePath))
	return filepath.Join(store.dir, hex.EncodeToString(digest[:])+".json")
}
