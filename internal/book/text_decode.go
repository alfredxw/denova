package book

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
)

func decodeNovelTextBytes(data []byte) (string, error) {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xef, 0xbb, 0xbf}) {
		if !utf8.Valid(data[3:]) {
			return "", fmt.Errorf("不支持的文本编码")
		}
		return string(data[3:]), nil
	}
	if len(data) >= 2 {
		if bytes.Equal(data[:2], []byte{0xff, 0xfe}) {
			return decodeTextBytesWith("utf-16le", data, textunicode.UTF16(textunicode.LittleEndian, textunicode.ExpectBOM).NewDecoder().Bytes)
		}
		if bytes.Equal(data[:2], []byte{0xfe, 0xff}) {
			return decodeTextBytesWith("utf-16be", data, textunicode.UTF16(textunicode.BigEndian, textunicode.ExpectBOM).NewDecoder().Bytes)
		}
	}
	if utf8.Valid(data) {
		text := string(data)
		if decodedTextPenalty(text) == 0 {
			return text, nil
		}
	}

	candidates := []struct {
		name   string
		decode func([]byte) ([]byte, error)
	}{
		{name: "gb18030", decode: simplifiedchinese.GB18030.NewDecoder().Bytes},
		{name: "utf-16le", decode: textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM).NewDecoder().Bytes},
		{name: "utf-16be", decode: textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM).NewDecoder().Bytes},
	}

	bestText := ""
	bestScore := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		text, err := decodeTextBytesWith(candidate.name, data, candidate.decode)
		if err != nil {
			continue
		}
		score := decodedTextPenalty(text)
		if score < bestScore {
			bestText = text
			bestScore = score
		}
	}
	if bestText == "" {
		return "", fmt.Errorf("不支持的文本编码")
	}
	return bestText, nil
}

func decodeTextBytesWith(_ string, data []byte, decode func([]byte) ([]byte, error)) (string, error) {
	decoded, err := decode(data)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(decoded) {
		return "", fmt.Errorf("decoded text is not utf-8")
	}
	return string(decoded), nil
}

func decodedTextPenalty(text string) int {
	score := 0
	for _, r := range text {
		if r == utf8.RuneError {
			score += 100
			continue
		}
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 {
			score += 20
		}
	}
	return score
}
