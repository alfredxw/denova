package character

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
)

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
