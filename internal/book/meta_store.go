package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StoredMeta is the v0.3.3 Book metadata representation. New metadata is a
// plain BookMeta at <Project StateRoot>/meta.json; this type exists only for
// the explicit release migration.
type StoredMeta struct {
	Path string `json:"path"`
	BookMeta
}

// MetaStore owns Project-state Book metadata and the bounded v0.3.3 migration
// source below book_meta. Final reads and writes never derive identity from a
// content path.
type MetaStore struct {
	dataRoot  string
	legacyDir string
}

func NewMetaStore(denovaDir string) *MetaStore {
	return &MetaStore{dataRoot: denovaDir, legacyDir: filepath.Join(denovaDir, "book_meta")}
}

func (store *MetaStore) Read(contentRoot, stateRoot string) (BookMeta, error) {
	data, err := os.ReadFile(store.metaPath(stateRoot))
	if err == nil {
		var meta BookMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return BookMeta{}, fmt.Errorf("decode Book metadata: %w", err)
		}
		return meta, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return BookMeta{}, fmt.Errorf("read Book metadata: %w", err)
	}
	return ReadBookMetaFromDir(contentRoot), nil
}

func (store *MetaStore) Write(contentRoot, stateRoot string, meta BookMeta) (BookMeta, error) {
	now := time.Now().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		if existing, readErr := store.Read(contentRoot, stateRoot); readErr == nil && existing.CreatedAt != "" {
			meta.CreatedAt = existing.CreatedAt
		} else {
			meta.CreatedAt = now
		}
	}
	meta.UpdatedAt = now
	if meta.Title == "" {
		meta.Title = filepath.Base(contentRoot)
	}
	if err := store.write(stateRoot, meta); err != nil {
		return BookMeta{}, err
	}
	return meta, nil
}

// MigrateLegacy copies the one matching v0.3.3 metadata record into final
// Project state. It never deletes or rewrites the release source and refuses
// ambiguous matches.
func (store *MetaStore) MigrateLegacy(contentRoot, stateRoot string) error {
	if _, err := os.Stat(store.metaPath(stateRoot)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	meta, found, err := store.findLegacy(contentRoot)
	if err != nil {
		return err
	}
	if !found {
		if _, statErr := os.Stat(filepath.Join(contentRoot, "book.json")); statErr == nil {
			meta, found = ReadBookMetaFromDir(contentRoot), true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	if !found {
		return nil
	}
	return store.write(stateRoot, meta)
}

func (store *MetaStore) findLegacy(contentRoot string) (BookMeta, bool, error) {
	absolute, err := filepath.Abs(contentRoot)
	if err != nil {
		return BookMeta{}, false, err
	}
	if data, readErr := os.ReadFile(store.legacyMetaPath(absolute)); readErr == nil {
		var stored StoredMeta
		if err := json.Unmarshal(data, &stored); err != nil {
			return BookMeta{}, false, fmt.Errorf("decode v0.3.3 Book metadata: %w", err)
		}
		return stored.BookMeta, true, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return BookMeta{}, false, readErr
	}

	entries, err := os.ReadDir(store.legacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return BookMeta{}, false, nil
	}
	if err != nil {
		return BookMeta{}, false, err
	}
	var matched *BookMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(store.legacyDir, entry.Name()))
		if readErr != nil {
			return BookMeta{}, false, readErr
		}
		var stored StoredMeta
		if json.Unmarshal(data, &stored) != nil || !store.legacyOwnerMatches(stored.Path, absolute) {
			continue
		}
		if matched != nil {
			return BookMeta{}, false, fmt.Errorf("multiple v0.3.3 metadata records match Book %s", contentRoot)
		}
		value := stored.BookMeta
		matched = &value
	}
	if matched == nil {
		return BookMeta{}, false, nil
	}
	return *matched, true, nil
}

func (store *MetaStore) legacyOwnerMatches(legacyPath, contentRoot string) bool {
	legacyPath = strings.TrimSpace(legacyPath)
	if legacyPath == "" {
		return false
	}
	if filepath.IsAbs(legacyPath) && strings.EqualFold(filepath.Clean(legacyPath), filepath.Clean(contentRoot)) {
		return true
	}
	relative, err := filepath.Rel(store.dataRoot, contentRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	suffix := "/" + filepath.ToSlash(relative)
	portableLegacy := strings.ReplaceAll(legacyPath, `\`, "/")
	return strings.HasSuffix(strings.ToLower(portableLegacy), strings.ToLower(suffix))
}

func (store *MetaStore) write(stateRoot string, meta BookMeta) error {
	metaPath := store.metaPath(stateRoot)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
		return fmt.Errorf("create Book metadata directory: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Book metadata: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(metaPath), ".meta-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, metaPath); err != nil {
		return err
	}
	committed = true
	return nil
}

func (store *MetaStore) metaPath(stateRoot string) string {
	return filepath.Join(stateRoot, "meta.json")
}

func (store *MetaStore) legacyMetaPath(absolutePath string) string {
	digest := sha256.Sum256([]byte(absolutePath))
	return filepath.Join(store.legacyDir, hex.EncodeToString(digest[:])+".json")
}
