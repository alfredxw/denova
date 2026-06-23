package book

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const chapterStatusFileName = "chapter_statuses.json"

type chapterStatusStore struct {
	Version  int             `json:"version"`
	Chapters map[string]bool `json:"chapters"`
}

// SetChapterConfirmed records whether a chapter has been manually confirmed as a finished chapter.
func (s *Service) SetChapterConfirmed(relPath string, confirmed bool) error {
	normalized, err := s.normalizeChapterPath(relPath)
	if err != nil {
		return err
	}

	store, err := s.readChapterStatusStore()
	if err != nil {
		return err
	}
	if store.Chapters == nil {
		store.Chapters = map[string]bool{}
	}
	if confirmed {
		store.Chapters[normalized] = true
	} else {
		delete(store.Chapters, normalized)
	}
	return s.writeChapterStatusStore(store)
}

func (s *Service) chapterConfirmedMap() map[string]bool {
	store, err := s.readChapterStatusStore()
	if err != nil || store.Chapters == nil {
		return map[string]bool{}
	}
	return store.Chapters
}

func (s *Service) normalizeChapterPath(relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("章节路径不能为空")
	}
	absPath, err := SafePath(s.workspace, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("章节路径不能是目录")
	}
	if !isChapterTextFile(info.Name()) {
		return "", errors.New("仅支持 md/txt 章节文件")
	}
	rel, err := filepath.Rel(s.workspace, absPath)
	if err != nil {
		return "", err
	}
	normalized := filepath.ToSlash(rel)
	if normalized == "chapters" || !strings.HasPrefix(normalized, "chapters/") {
		return "", errors.New("只能确认 chapters/ 下的章节")
	}
	return normalized, nil
}

func (s *Service) readChapterStatusStore() (chapterStatusStore, error) {
	path := s.chapterStatusPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return chapterStatusStore{Version: 1, Chapters: map[string]bool{}}, nil
	}
	if err != nil {
		return chapterStatusStore{}, err
	}
	var store chapterStatusStore
	if err := json.Unmarshal(data, &store); err != nil {
		return chapterStatusStore{}, err
	}
	if store.Version == 0 {
		store.Version = 1
	}
	if store.Chapters == nil {
		store.Chapters = map[string]bool{}
	}
	return store, nil
}

func (s *Service) writeChapterStatusStore(store chapterStatusStore) error {
	store.Version = 1
	if store.Chapters == nil {
		store.Chapters = map[string]bool{}
	}
	path := s.chapterStatusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Service) chapterStatusPath() string {
	return filepath.Join(s.workspace, ".nova", chapterStatusFileName)
}

func chapterStatus(words int, confirmed bool) string {
	if words == 0 {
		return "空章"
	}
	if confirmed {
		return "成章"
	}
	return "初稿"
}
