package book

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"denova/internal/workspacepath"
)

const tavernCardCoverPath = "assets/image/cover.png"
const interactiveOpeningPresetPath = "setting/interactive-openings.json"

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// CharacterCardImportResult 描述酒馆角色卡导入结果。
type CharacterCardImportResult struct {
	Name                 string                           `json:"name"`
	TargetPath           string                           `json:"target_path"`
	EntryCount           int                              `json:"entry_count"`
	ItemCount            int                              `json:"item_count"`
	ItemIDs              []string                         `json:"item_ids"`
	CoverPath            string                           `json:"cover_path,omitempty"`
	OpeningPresetPath    string                           `json:"opening_preset_path,omitempty"`
	OpeningPresetCount   int                              `json:"opening_preset_count"`
	UserPlaceholderFound bool                             `json:"user_placeholder_found"`
	UserCharacterName    string                           `json:"user_character_name,omitempty"`
	Workspace            string                           `json:"workspace,omitempty"`
	BookMeta             *BookMeta                        `json:"book_meta,omitempty"`
	Compatibility        CharacterCardCompatibilityReport `json:"compatibility"`
	Message              string                           `json:"message"`
	ResidentLoreBytes    int                              `json:"resident_lore_bytes"`
	ClassificationMode   string                           `json:"classification_mode"`
	ClassificationCounts map[string]int                   `json:"classification_counts"`
	UncertainTypeCount   int                              `json:"uncertain_type_count"`
}

// CharacterCardPreview 描述酒馆角色卡预览信息，解析但不写入 workspace。
type CharacterCardPreview struct {
	Name                  string                           `json:"name"`
	EntryCount            int                              `json:"entry_count"`
	Tags                  []string                         `json:"tags"`
	OpeningPresetCount    int                              `json:"opening_preset_count"`
	UserPlaceholderFound  bool                             `json:"user_placeholder_found"`
	WillImportCover       bool                             `json:"will_import_cover"`
	Compatibility         CharacterCardCompatibilityReport `json:"compatibility"`
	EnabledEntryCount     int                              `json:"enabled_entry_count"`
	DisabledEntryCount    int                              `json:"disabled_entry_count"`
	ResidentEntryCount    int                              `json:"resident_entry_count"`
	ResidentEntryBytes    int                              `json:"resident_entry_bytes"`
	ResidentLoreBytes     int                              `json:"resident_lore_bytes"`
	AutoEntryCount        int                              `json:"auto_entry_count"`
	RemovedRuntimeCount   int                              `json:"removed_runtime_entry_count"`
	SanitizedMixedCount   int                              `json:"sanitized_mixed_entry_count"`
	OpeningTruncatedCount int                              `json:"opening_truncated_count"`
	ResidentLoreWarning   bool                             `json:"resident_lore_warning"`
	ResidentLoreWarningKB int                              `json:"resident_lore_warning_threshold_kb"`
	ClassificationMode    string                           `json:"classification_mode"`
	ClassificationCounts  map[string]int                   `json:"classification_counts"`
	UncertainTypeCount    int                              `json:"uncertain_type_count"`
}

// CharacterCardCompatibilityReport reports Denova capabilities rather than
// exposing Tavern's runtime field vocabulary to users.
type CharacterCardCompatibilityReport struct {
	Capabilities        []string `json:"capabilities"`
	SanitizedRuntime    []string `json:"sanitized_runtime"`
	DiscardedExtensions []string `json:"discarded_extensions"`
	Warnings            []string `json:"warnings"`
	IgnoredLoadingRules bool     `json:"ignored_loading_rules"`
}

type CharacterCardImportOptions struct {
	UserCharacterName  string
	ClassificationMode string
	ClassifyLore       LoreSemanticClassifier
}

type tavernCard struct {
	Spec                    string               `json:"spec"`
	SpecVersion             string               `json:"spec_version"`
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	CreatorComment          string               `json:"creatorcomment"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Avatar                  string               `json:"avatar"`
	Talkativeness           any                  `json:"talkativeness"`
	Fav                     any                  `json:"fav"`
	CreateDate              any                  `json:"create_date"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
	Data                    *tavernCardData      `json:"data"`
}

type tavernCardData struct {
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Creator                 string               `json:"creator"`
	CharacterVersion        string               `json:"character_version"`
	Extensions              map[string]any       `json:"extensions"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
}

type tavernCharacterBook struct {
	Name    string            `json:"name"`
	Entries []tavernBookEntry `json:"entries"`
}

type tavernBookEntry struct {
	ID                  int      `json:"id"`
	Keys                []string `json:"keys"`
	SecondaryKeys       []string `json:"secondary_keys"`
	Comment             string   `json:"comment"`
	Content             string   `json:"content"`
	Constant            bool     `json:"constant"`
	Selective           bool     `json:"selective"`
	Enabled             *bool    `json:"enabled"`
	Position            any      `json:"position"`
	InsertionOrder      int      `json:"insertion_order"`
	SelectiveLogic      any      `json:"selectiveLogic"`
	Probability         any      `json:"probability"`
	UseProbability      bool     `json:"useProbability"`
	Group               string   `json:"group"`
	Depth               any      `json:"depth"`
	Role                any      `json:"role"`
	PreventRecursion    bool     `json:"preventRecursion"`
	DelayUntilRecursion bool     `json:"delayUntilRecursion"`
	Sticky              any      `json:"sticky"`
	Cooldown            any      `json:"cooldown"`
	Vectorized          any      `json:"vectorized"`
}

type normalizedTavernCard struct {
	Spec                    string
	SpecVersion             string
	Name                    string
	Description             string
	Personality             string
	Scenario                string
	FirstMes                string
	MesExample              string
	CreatorNotes            string
	CreatorComment          string
	SystemPrompt            string
	PostHistoryInstructions string
	Creator                 string
	CharacterVersion        string
	Avatar                  string
	Talkativeness           any
	Fav                     any
	CreateDate              any
	Extensions              map[string]any
	Tags                    []string
	AlternateGreetings      []string
	CharacterBook           *tavernCharacterBook
	IsPNG                   bool
	HasUserPlaceholder      bool
	Warnings                []string
}

type pngTextChunk struct {
	Keyword string
	Text    string
}

// ImportTavernCharacterCard 将 SillyTavern 酒馆角色卡（PNG 或 JSON）转换为互动资料库条目。
func (s *Service) ImportTavernCharacterCard(filename string, data []byte, opts ...CharacterCardImportOptions) (CharacterCardImportResult, error) {
	card, err := parseTavernCharacterCard(filename, data)
	if err != nil {
		return CharacterCardImportResult{}, err
	}
	options := mergeCharacterCardImportOptions(opts...)
	loreStore := NewLoreStore(s.workspace)
	existingItems, err := loreStore.ListAll()
	if err != nil {
		return CharacterCardImportResult{}, err
	}
	coverPath := ""
	if card.IsPNG {
		coverPath = tavernCardCoverPath
	}
	ops, importStats := buildTavernCardLoreOperations(card, filename, coverPath, options.UserCharacterName, newLoreNameAllocator(existingItems))
	importStats.ClassificationMode = normalizeLoreClassificationMode(options.ClassificationMode)
	if importStats.ClassificationMode == LoreClassificationModeSemantic && options.ClassifyLore != nil {
		if err := applySemanticTavernLoreClassification(ops, &importStats, options.ClassifyLore); err != nil {
			importStats.Warnings = append(importStats.Warnings, "语义资料分类失败，已保留名称优先的本地分类结果："+err.Error())
		}
	}
	// Semantic classification can be slow and performs no local writes. Take
	// the rollback snapshot only after it finishes so a later rollback cannot
	// overwrite user edits made while the model request was running.
	snapshots, err := snapshotCharacterCardImportFiles(s.workspace)
	if err != nil {
		return CharacterCardImportResult{}, err
	}
	rollback := func(cause error) (CharacterCardImportResult, error) {
		if rollbackErr := restoreCharacterCardImportFiles(snapshots); rollbackErr != nil {
			return CharacterCardImportResult{}, fmt.Errorf("%w；回滚导入文件失败: %v", cause, rollbackErr)
		}
		return CharacterCardImportResult{}, cause
	}
	coverPath, err = s.importTavernCardCover(card, data)
	if err != nil {
		return rollback(err)
	}
	openingCount, err := s.importTavernCardOpeningPresets(card)
	if err != nil {
		return rollback(err)
	}
	applyResult, err := loreStore.ApplyOperations(fmt.Sprintf("导入酒馆角色卡「%s」", card.Name), ops)
	if err != nil {
		return rollback(err)
	}

	itemIDs := make([]string, 0, len(applyResult.Created))
	for _, item := range applyResult.Created {
		itemIDs = append(itemIDs, item.ID)
	}
	result := CharacterCardImportResult{
		Name:                 card.Name,
		TargetPath:           loreItemsRelPath(s.workspace),
		EntryCount:           characterBookEntryCount(card.CharacterBook),
		ItemCount:            len(itemIDs),
		ItemIDs:              itemIDs,
		CoverPath:            coverPath,
		OpeningPresetPath:    openingPresetPath(openingCount),
		OpeningPresetCount:   openingCount,
		UserPlaceholderFound: card.HasUserPlaceholder,
		UserCharacterName:    tavernUserCharacterName(card, options.UserCharacterName),
		Compatibility:        tavernCardCompatibility(card),
		Message:              fmt.Sprintf("已导入酒馆角色卡「%s」到互动资料库", card.Name),
		ResidentLoreBytes:    importStats.ResidentLoreBytes,
		ClassificationMode:   importStats.ClassificationMode,
		ClassificationCounts: cloneLoreTypeCounts(importStats.ClassificationCounts),
		UncertainTypeCount:   importStats.UncertainTypeCount,
	}
	result.Compatibility.Warnings = append(result.Compatibility.Warnings, importStats.Warnings...)
	return result, nil
}

func PreviewTavernCharacterCard(filename string, data []byte) (CharacterCardPreview, error) {
	card, err := parseTavernCharacterCard(filename, data)
	if err != nil {
		return CharacterCardPreview{}, err
	}
	_, stats := buildTavernCardLoreOperations(card, filename, "", "玩家角色", newLoreNameAllocator(nil))
	return CharacterCardPreview{
		Name:                  card.Name,
		EntryCount:            characterBookEntryCount(card.CharacterBook),
		Tags:                  tavernCardTags(card.Tags...),
		OpeningPresetCount:    tavernCardOpeningPresetCount(card),
		UserPlaceholderFound:  card.HasUserPlaceholder,
		WillImportCover:       card.IsPNG,
		Compatibility:         tavernCardCompatibility(card),
		EnabledEntryCount:     stats.EnabledEntryCount,
		DisabledEntryCount:    stats.DisabledEntryCount,
		ResidentEntryCount:    stats.ResidentEntryCount,
		ResidentEntryBytes:    stats.ResidentEntryBytes,
		ResidentLoreBytes:     stats.ResidentLoreBytes,
		AutoEntryCount:        stats.AutoEntryCount,
		RemovedRuntimeCount:   stats.RemovedRuntimeCount,
		SanitizedMixedCount:   stats.SanitizedMixedCount,
		OpeningTruncatedCount: tavernCardOpeningTruncatedCount(card),
		ResidentLoreWarning:   stats.ResidentLoreBytes > ResidentLoreWarningBytes,
		ResidentLoreWarningKB: bytesToKB(ResidentLoreWarningBytes),
		ClassificationMode:    LoreClassificationModeHeuristic,
		ClassificationCounts:  cloneLoreTypeCounts(stats.ClassificationCounts),
		UncertainTypeCount:    stats.UncertainTypeCount,
	}, nil
}

func parseTavernCharacterCard(filename string, data []byte) (normalizedTavernCard, error) {
	if len(data) == 0 {
		return normalizedTavernCard{}, errors.New("角色卡文件为空")
	}

	var rawJSON []byte
	var parseWarnings []string
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case bytes.HasPrefix(data, pngSignature) || ext == ".png":
		payload, warnings, err := extractTavernPayloadFromPNG(data)
		if err != nil {
			return normalizedTavernCard{}, err
		}
		rawJSON = payload
		parseWarnings = warnings
	case ext == ".json" || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")):
		rawJSON = bytes.TrimSpace(data)
	default:
		return normalizedTavernCard{}, errors.New("仅支持导入 PNG 或 JSON 格式的酒馆角色卡")
	}

	card, err := decodeTavernCardJSON(rawJSON)
	if err != nil {
		return normalizedTavernCard{}, err
	}
	if strings.TrimSpace(card.Name) == "" {
		return normalizedTavernCard{}, errors.New("角色卡缺少 name 字段")
	}
	card.IsPNG = bytes.HasPrefix(data, pngSignature) || ext == ".png"
	card.Warnings = append(card.Warnings, parseWarnings...)
	card.HasUserPlaceholder = tavernCardContainsUserPlaceholder(card)
	return card, nil
}

func extractTavernPayloadFromPNG(data []byte) ([]byte, []string, error) {
	chunks, err := extractPNGTextChunks(data)
	if err != nil {
		return nil, nil, err
	}
	encoded := map[string]string{}
	for _, chunk := range chunks {
		if chunk.Keyword != "chara" && chunk.Keyword != "ccv3" {
			continue
		}
		encoded[chunk.Keyword] = chunk.Text
	}
	if text, ok := encoded["ccv3"]; ok {
		payload, err := decodeTavernTextPayload(text)
		if err != nil {
			return nil, nil, fmt.Errorf("解析 PNG 角色卡 ccv3 元数据失败: %w", err)
		}
		warnings := []string{}
		if legacyText, exists := encoded["chara"]; exists {
			legacy, legacyErr := decodeTavernTextPayload(legacyText)
			if legacyErr != nil || !jsonPayloadEqual(payload, legacy) {
				warnings = append(warnings, "ccv3_conflict")
			}
		}
		return payload, warnings, nil
	}
	if text, ok := encoded["chara"]; ok {
		payload, err := decodeTavernTextPayload(text)
		if err != nil {
			return nil, nil, fmt.Errorf("解析 PNG 角色卡 chara 元数据失败: %w", err)
		}
		return payload, nil, nil
	}
	return nil, nil, errors.New("PNG 中未找到酒馆角色卡 ccv3 或 chara 元数据")
}

func jsonPayloadEqual(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil {
		leftJSON, _ := json.Marshal(a)
		rightJSON, _ := json.Marshal(b)
		return bytes.Equal(leftJSON, rightJSON)
	}
	return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
}

func extractPNGTextChunks(data []byte) ([]pngTextChunk, error) {
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, errors.New("不是有效的 PNG 文件")
	}
	var chunks []pngTextChunk
	offset := len(pngSignature)
	for offset+12 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])
		offset += 8
		if length < 0 || offset+length+4 > len(data) {
			return nil, errors.New("PNG 数据块长度不合法")
		}
		chunkData := data[offset : offset+length]
		offset += length + 4

		switch chunkType {
		case "tEXt":
			chunk, ok := parsePNGTextChunk(chunkData)
			if ok {
				chunks = append(chunks, chunk)
			}
		case "zTXt":
			chunk, err := parsePNGCompressedTextChunk(chunkData)
			if err != nil {
				return nil, err
			}
			if chunk.Keyword != "" {
				chunks = append(chunks, chunk)
			}
		case "iTXt":
			chunk, err := parsePNGInternationalTextChunk(chunkData)
			if err != nil {
				return nil, err
			}
			if chunk.Keyword != "" {
				chunks = append(chunks, chunk)
			}
		case "IEND":
			return chunks, nil
		}
	}
	return chunks, nil
}

func parsePNGTextChunk(data []byte) (pngTextChunk, bool) {
	idx := bytes.IndexByte(data, 0)
	if idx <= 0 {
		return pngTextChunk{}, false
	}
	return pngTextChunk{
		Keyword: string(data[:idx]),
		Text:    string(data[idx+1:]),
	}, true
}

func parsePNGCompressedTextChunk(data []byte) (pngTextChunk, error) {
	idx := bytes.IndexByte(data, 0)
	if idx <= 0 || idx+2 > len(data) {
		return pngTextChunk{}, nil
	}
	if data[idx+1] != 0 {
		return pngTextChunk{}, errors.New("PNG zTXt 使用了不支持的压缩方法")
	}
	text, err := inflateZlib(data[idx+2:])
	if err != nil {
		return pngTextChunk{}, err
	}
	return pngTextChunk{Keyword: string(data[:idx]), Text: text}, nil
}

func parsePNGInternationalTextChunk(data []byte) (pngTextChunk, error) {
	keywordEnd := bytes.IndexByte(data, 0)
	if keywordEnd <= 0 || keywordEnd+3 > len(data) {
		return pngTextChunk{}, nil
	}
	keyword := string(data[:keywordEnd])
	compressionFlag := data[keywordEnd+1]
	compressionMethod := data[keywordEnd+2]
	if compressionMethod != 0 {
		return pngTextChunk{}, errors.New("PNG iTXt 使用了不支持的压缩方法")
	}
	rest := data[keywordEnd+3:]
	languageEnd := bytes.IndexByte(rest, 0)
	if languageEnd < 0 {
		return pngTextChunk{}, nil
	}
	rest = rest[languageEnd+1:]
	translatedEnd := bytes.IndexByte(rest, 0)
	if translatedEnd < 0 {
		return pngTextChunk{}, nil
	}
	textBytes := rest[translatedEnd+1:]
	if compressionFlag == 1 {
		text, err := inflateZlib(textBytes)
		if err != nil {
			return pngTextChunk{}, err
		}
		return pngTextChunk{Keyword: keyword, Text: text}, nil
	}
	return pngTextChunk{Keyword: keyword, Text: string(textBytes)}, nil
}

func inflateZlib(data []byte) (string, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解压 PNG 文本块失败: %w", err)
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取 PNG 文本块失败: %w", err)
	}
	return string(out), nil
}

func decodeTavernTextPayload(text string) ([]byte, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	compacted := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, trimmed)
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(compacted)
		if err == nil {
			return bytes.TrimSpace(decoded), nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func decodeTavernCardJSON(data []byte) (normalizedTavernCard, error) {
	var raw tavernCard
	if err := json.Unmarshal(data, &raw); err != nil {
		return normalizedTavernCard{}, fmt.Errorf("解析角色卡 JSON 失败: %w", err)
	}

	card := normalizedTavernCard{
		Spec:                    raw.Spec,
		SpecVersion:             raw.SpecVersion,
		Name:                    raw.Name,
		Description:             raw.Description,
		Personality:             raw.Personality,
		Scenario:                raw.Scenario,
		FirstMes:                raw.FirstMes,
		MesExample:              raw.MesExample,
		CreatorNotes:            raw.CreatorNotes,
		CreatorComment:          raw.CreatorComment,
		SystemPrompt:            raw.SystemPrompt,
		PostHistoryInstructions: raw.PostHistoryInstructions,
		Avatar:                  raw.Avatar,
		Talkativeness:           raw.Talkativeness,
		Fav:                     raw.Fav,
		CreateDate:              raw.CreateDate,
		Tags:                    raw.Tags,
		AlternateGreetings:      raw.AlternateGreetings,
		CharacterBook:           raw.CharacterBook,
	}
	if raw.Data != nil {
		card.Name = firstNonEmpty(raw.Data.Name, card.Name)
		card.Description = firstNonEmpty(raw.Data.Description, card.Description)
		card.Personality = firstNonEmpty(raw.Data.Personality, card.Personality)
		card.Scenario = firstNonEmpty(raw.Data.Scenario, card.Scenario)
		card.FirstMes = firstNonEmpty(raw.Data.FirstMes, card.FirstMes)
		card.MesExample = firstNonEmpty(raw.Data.MesExample, card.MesExample)
		card.CreatorNotes = firstNonEmpty(raw.Data.CreatorNotes, card.CreatorNotes)
		card.SystemPrompt = firstNonEmpty(raw.Data.SystemPrompt, card.SystemPrompt)
		card.PostHistoryInstructions = firstNonEmpty(raw.Data.PostHistoryInstructions, card.PostHistoryInstructions)
		card.Creator = strings.TrimSpace(raw.Data.Creator)
		card.CharacterVersion = strings.TrimSpace(raw.Data.CharacterVersion)
		if len(raw.Data.Extensions) > 0 {
			card.Extensions = raw.Data.Extensions
		}
		if len(raw.Data.Tags) > 0 {
			card.Tags = raw.Data.Tags
		}
		if len(raw.Data.AlternateGreetings) > 0 {
			card.AlternateGreetings = raw.Data.AlternateGreetings
		}
		if raw.Data.CharacterBook != nil {
			card.CharacterBook = raw.Data.CharacterBook
		}
	}
	card.Name = strings.TrimSpace(card.Name)
	return card, nil
}

type tavernImportStats struct {
	EnabledEntryCount    int
	DisabledEntryCount   int
	ResidentEntryCount   int
	ResidentEntryBytes   int
	ResidentLoreBytes    int
	AutoEntryCount       int
	RemovedRuntimeCount  int
	SanitizedMixedCount  int
	Warnings             []string
	ClassificationMode   string
	ClassificationCounts map[string]int
	UncertainTypeCount   int
	UncertainOpIndexes   []int
}

func buildTavernCardLoreOperations(card normalizedTavernCard, source, coverPath, userCharacterName string, names *loreNameAllocator) ([]LoreOperation, tavernImportStats) {
	if names == nil {
		names = newLoreNameAllocator(nil)
	}
	stats := tavernImportStats{ClassificationMode: LoreClassificationModeHeuristic, ClassificationCounts: map[string]int{}}
	cardLoreName := names.Claim(card.Name)
	cardContent := renderTavernCardLoreContent(card, coverPath)
	cardKeywords := tavernCardTags(card.Tags...)
	ops := []LoreOperation{
		{
			Op: "create",
			Item: LoreItemInput{
				Enabled:          loreEnabledPtr(true),
				Type:             "character",
				TypeSource:       LoreTypeSourceHeuristic,
				Name:             cardLoreName,
				Importance:       "major",
				Tags:             tavernCardTags(append([]string{"酒馆角色卡", card.Name}, card.Tags...)...),
				BriefDescription: tavernLoreSearchBrief("character", cardLoreName, cardKeywords),
				Keywords:         cardKeywords,
				LoadMode:         LoreLoadModeResident,
				Content:          cardContent,
				Provenance:       tavernLoreProvenance("tavern_character_card", source, "character", card),
			},
		},
	}
	stats.ResidentLoreBytes += len([]byte(cardContent))
	if card.HasUserPlaceholder {
		name := names.Claim(tavernUserCharacterName(card, userCharacterName))
		content := renderTavernUserPlaceholderLoreContent(card, name)
		ops = append(ops, LoreOperation{
			Op: "create",
			Item: LoreItemInput{
				Enabled:          loreEnabledPtr(true),
				Type:             "character",
				TypeSource:       LoreTypeSourceHeuristic,
				Name:             name,
				Importance:       "major",
				Tags:             tavernCardTags("酒馆角色卡", "{{user}}", "玩家角色"),
				BriefDescription: tavernLoreSearchBrief("character", name, []string{"{{user}}", card.Name}),
				Keywords:         tavernCardTags("{{user}}", card.Name),
				LoadMode:         LoreLoadModeResident,
				Content:          content,
				Provenance:       tavernLoreProvenance("tavern_character_card", source, "user", card),
			},
		})
		stats.ResidentLoreBytes += len([]byte(content))
	}
	if card.CharacterBook == nil {
		return ops, stats
	}
	for i, entry := range card.CharacterBook.Entries {
		if entry.Enabled != nil && !*entry.Enabled {
			stats.DisabledEntryCount++
		} else {
			stats.EnabledEntryCount++
		}
		sanitized := sanitizeTavernBookEntry(entry)
		if sanitized.Removed {
			stats.RemovedRuntimeCount++
			continue
		}
		if sanitized.MixedCleaned {
			stats.SanitizedMixedCount++
		}
		title := names.Claim(tavernBookEntryTitle(entry, i))
		content := sanitized.Content
		loadMode := LoreLoadModeAuto
		if entry.Constant {
			loadMode = LoreLoadModeResident
			stats.ResidentEntryCount++
			if entry.Enabled == nil || *entry.Enabled {
				stats.ResidentLoreBytes += len([]byte(content))
				stats.ResidentEntryBytes += len([]byte(content))
			}
		} else {
			stats.AutoEntryCount++
		}
		keywords := tavernCardTags(append(append([]string{}, entry.Keys...), entry.SecondaryKeys...)...)
		suggestion := ClassifyLoreItemHeuristic(LoreClassificationInput{
			Name: title, Tags: []string{"酒馆世界书", card.Name}, Keywords: keywords, Content: content,
		})
		itemType := suggestion.Type
		tags := tavernCardTags("酒馆世界书", card.Name)
		ops = append(ops, LoreOperation{
			Op: "create",
			Item: LoreItemInput{
				Enabled:          tavernBookEntryEnabled(entry),
				Type:             itemType,
				TypeSource:       LoreTypeSourceHeuristic,
				Name:             title,
				Importance:       "important",
				Tags:             tags,
				Keywords:         keywords,
				BriefDescription: tavernLoreSearchBrief(itemType, title, keywords),
				LoadMode:         loadMode,
				Content:          content,
				Provenance:       tavernLoreProvenance("tavern_worldbook_entry", source, tavernEntryRecordID(entry, i), entry),
			},
		})
		stats.ClassificationCounts[itemType]++
		if suggestion.Confidence != LoreClassificationConfidenceHigh {
			stats.UncertainTypeCount++
			stats.UncertainOpIndexes = append(stats.UncertainOpIndexes, len(ops)-1)
		}
	}
	return ops, stats
}

func tavernLoreSearchBrief(itemType, name string, keywords []string) string {
	subject := fmt.Sprintf("%s「%s」", loreTypeLabel(itemType), strings.TrimSpace(name))
	keywords = tavernCardTags(keywords...)
	if len(keywords) == 0 {
		return subject + "；无额外搜索关键词，可按名称读取正文。"
	}
	return truncateRunes(subject+"；搜索关键词："+strings.Join(keywords, "、")+"。", 240)
}

func renderTavernCardLoreContent(card normalizedTavernCard, coverPath string) string {
	var sb strings.Builder
	if coverPath != "" {
		sb.WriteString("![")
		sb.WriteString(card.Name)
		sb.WriteString("](")
		sb.WriteString(coverPath)
		sb.WriteString(")\n\n")
	}

	writeMarkdownSection(&sb, "角色描述", sanitizeTavernNaturalLanguage(card.Description))
	writeMarkdownSection(&sb, "性格", sanitizeTavernNaturalLanguage(card.Personality))
	writeMarkdownSection(&sb, "场景", sanitizeTavernNaturalLanguage(card.Scenario))
	writeMarkdownSection(&sb, "对话示例", sanitizeTavernNaturalLanguage(card.MesExample))
	writeMarkdownSection(&sb, "作者备注", sanitizeTavernRuntimeProneField(card.CreatorNotes))
	writeMarkdownSection(&sb, "创建者备注", sanitizeTavernRuntimeProneField(card.CreatorComment))
	writeMarkdownSection(&sb, "系统提示", sanitizeTavernRuntimeProneField(card.SystemPrompt))
	writeMarkdownSection(&sb, "历史后置提示", sanitizeTavernRuntimeProneField(card.PostHistoryInstructions))
	return strings.TrimSpace(sb.String())
}

func renderTavernUserPlaceholderLoreContent(card normalizedTavernCard, name string) string {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString(" 是与 ")
	sb.WriteString(card.Name)
	sb.WriteString(" 互动的玩家角色。请补充姓名、身份、关系与需要保持稳定的个人事实。\n")
	return strings.TrimSpace(sb.String())
}

func tavernEntryRecordID(entry tavernBookEntry, index int) string {
	if entry.ID != 0 {
		return fmt.Sprintf("%d", entry.ID)
	}
	return fmt.Sprintf("entry-%d", index+1)
}

func tavernLoreProvenance(kind, source, recordID string, record any) *LoreProvenance {
	data, _ := json.Marshal(record)
	sum := sha256.Sum256(data)
	return &LoreProvenance{Kind: kind, SourceName: source, SourceRecordID: recordID, SourceHash: fmt.Sprintf("%x", sum[:])}
}

func writeMarkdownSection(sb *strings.Builder, title, content string) {
	content = normalizeCardText(content)
	if content == "" {
		return
	}
	sb.WriteString("### ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n")
}

func tavernBookEntryTitle(entry tavernBookEntry, index int) string {
	return firstNonEmpty(entry.Comment, strings.Join(entry.Keys, "、"), fmt.Sprintf("条目 %d", index+1))
}

func tavernCardTags(values ...string) []string {
	tags := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		tags = append(tags, value)
	}
	return tags
}

func normalizeCardText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func characterBookEntryCount(book *tavernCharacterBook) int {
	if book == nil {
		return 0
	}
	return len(book.Entries)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeCharacterCardImportOptions(opts ...CharacterCardImportOptions) CharacterCardImportOptions {
	var merged CharacterCardImportOptions
	for _, opt := range opts {
		if name := strings.TrimSpace(opt.UserCharacterName); name != "" {
			merged.UserCharacterName = name
		}
		if mode := strings.TrimSpace(opt.ClassificationMode); mode != "" {
			merged.ClassificationMode = mode
		}
		if opt.ClassifyLore != nil {
			merged.ClassifyLore = opt.ClassifyLore
		}
	}
	merged.ClassificationMode = normalizeLoreClassificationMode(merged.ClassificationMode)
	return merged
}

func bytesToKB(value int) int {
	if value <= 0 {
		return 0
	}
	return (value + 1023) / 1024
}

func tavernUserCharacterName(card normalizedTavernCard, name string) string {
	if !card.HasUserPlaceholder {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "玩家角色"
	}
	return name
}

func tavernBookEntryEnabled(entry tavernBookEntry) *bool {
	if entry.Enabled == nil {
		return loreEnabledPtr(true)
	}
	return loreEnabledPtr(*entry.Enabled)
}

func loreEnabledPtr(enabled bool) *bool {
	return &enabled
}

func loreItemsRelPath(workspace string) string {
	return workspacepath.Rel(workspace, "lore", "items.json")
}
