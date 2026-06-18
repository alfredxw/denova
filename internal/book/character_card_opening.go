package book

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type openingPresetFile struct {
	Version int             `json:"version"`
	Presets []openingPreset `json:"presets"`
}

type openingPreset struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (s *Service) importTavernCardCover(card normalizedTavernCard, data []byte) (string, error) {
	if !card.IsPNG {
		return "", nil
	}
	absPath, err := SafePath(s.workspace, tavernCardCoverPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("创建封面目录失败: %w", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", fmt.Errorf("写入酒馆角色卡封面失败: %w", err)
	}
	return tavernCardCoverPath, nil
}

func (s *Service) importTavernCardOpeningPresets(card normalizedTavernCard) (int, error) {
	presets := tavernCardOpeningPresets(card)
	if len(presets) == 0 {
		return 0, nil
	}
	absPath, err := SafePath(s.workspace, interactiveOpeningPresetPath)
	if err != nil {
		return 0, err
	}
	existing, err := readOpeningPresetFile(absPath)
	if err != nil {
		return 0, err
	}
	next := mergeOpeningPresets(existing, presets)
	out, err := json.MarshalIndent(openingPresetFile{Version: 1, Presets: next}, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("序列化互动开场白预设失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return 0, fmt.Errorf("创建互动开场白预设目录失败: %w", err)
	}
	if err := os.WriteFile(absPath, append(out, '\n'), 0o644); err != nil {
		return 0, fmt.Errorf("写入互动开场白预设失败: %w", err)
	}
	return len(presets), nil
}

func readOpeningPresetFile(absPath string) ([]openingPreset, error) {
	data, err := os.ReadFile(absPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取互动开场白预设失败: %w", err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var file openingPresetFile
	if err := json.Unmarshal(trimmed, &file); err == nil {
		return normalizeOpeningPresets(file.Presets), nil
	}
	var list []openingPreset
	if err := json.Unmarshal(trimmed, &list); err == nil {
		return normalizeOpeningPresets(list), nil
	}
	return []openingPreset{{ID: "legacy", Title: "默认开场白", Content: truncateTavernOpeningText(string(trimmed))}}, nil
}

func mergeOpeningPresets(existing, imported []openingPreset) []openingPreset {
	importedIDs := make(map[string]bool, len(imported))
	for _, preset := range imported {
		importedIDs[preset.ID] = true
	}
	next := make([]openingPreset, 0, len(existing)+len(imported))
	for _, preset := range existing {
		if importedIDs[preset.ID] {
			continue
		}
		next = append(next, preset)
	}
	next = append(next, imported...)
	return normalizeOpeningPresets(next)
}

func tavernCardOpeningPresets(card normalizedTavernCard) []openingPreset {
	baseID := "tavern-" + sanitizeOpeningPresetID(card.Name)
	if baseID == "tavern-" {
		baseID = "tavern-card"
	}
	presets := make([]openingPreset, 0, 1+len(card.AlternateGreetings))
	if text := truncateTavernOpeningText(card.FirstMes); text != "" {
		presets = append(presets, openingPreset{
			ID:      baseID + "-main",
			Title:   card.Name + " · 主开场白",
			Content: text,
		})
	}
	for i, greeting := range card.AlternateGreetings {
		text := truncateTavernOpeningText(greeting)
		if text == "" {
			continue
		}
		presets = append(presets, openingPreset{
			ID:      fmt.Sprintf("%s-alt-%d", baseID, i+1),
			Title:   fmt.Sprintf("%s · 备用开场白 %d", card.Name, i+1),
			Content: text,
		})
	}
	return normalizeOpeningPresets(presets)
}

func normalizeOpeningPresets(presets []openingPreset) []openingPreset {
	next := make([]openingPreset, 0, len(presets))
	seen := map[string]bool{}
	for i, preset := range presets {
		title := strings.TrimSpace(preset.Title)
		content := truncateTavernOpeningText(preset.Content)
		if title == "" && content == "" {
			continue
		}
		id := sanitizeOpeningPresetID(preset.ID)
		if id == "" {
			id = fmt.Sprintf("opening-%d", i+1)
		}
		baseID := id
		for suffix := 2; seen[id]; suffix++ {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		seen[id] = true
		if title == "" {
			title = fmt.Sprintf("开场白 %d", len(next)+1)
		}
		next = append(next, openingPreset{ID: id, Title: truncateTavernOpeningTitle(title), Content: content})
	}
	return next
}

func tavernCardOpeningPresetCount(card normalizedTavernCard) int {
	return len(tavernCardOpeningPresets(card))
}

func openingPresetPath(count int) string {
	if count == 0 {
		return ""
	}
	return interactiveOpeningPresetPath
}

func truncateTavernOpeningText(text string) string {
	text = normalizeCardText(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= 4000 {
		return text
	}
	return string(runes[:4000])
}

func truncateTavernOpeningTitle(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= 80 {
		return text
	}
	return string(runes[:80])
}

func sanitizeOpeningPresetID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var sb strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			lastDash = false
			continue
		}
		if r > 127 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			sb.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			sb.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}
