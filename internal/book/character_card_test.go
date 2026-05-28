package book

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTavernCharacterCardJSONV2(t *testing.T) {
	raw := []byte(`{
		"spec": "chara_card_v2",
		"spec_version": "2.0",
		"data": {
			"name": "林青",
			"description": "剑修",
			"personality": "冷静",
			"character_book": {
				"name": "林青世界书",
				"entries": [
					{"keys": ["宗门"], "comment": "出身", "content": "青岚宗内门弟子", "enabled": true}
				]
			}
		}
	}`)

	card, err := parseTavernCharacterCard("linqing.json", raw)
	if err != nil {
		t.Fatalf("解析 JSON 角色卡失败: %v", err)
	}
	if card.Name != "林青" {
		t.Fatalf("角色名不符合预期: %q", card.Name)
	}
	if characterBookEntryCount(card.CharacterBook) != 1 {
		t.Fatalf("世界书条目数不符合预期: %#v", card.CharacterBook)
	}
}

func TestParseTavernCharacterCardPNGTextChunk(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(`{"name":"许眠","description":"医生"}`))
	png := makeTestPNGTextChunk("chara", payload)

	card, err := parseTavernCharacterCard("xumian.png", png)
	if err != nil {
		t.Fatalf("解析 PNG 角色卡失败: %v", err)
	}
	if card.Name != "许眠" || card.Description != "医生" {
		t.Fatalf("PNG 角色卡内容不符合预期: %#v", card)
	}
}

func TestServiceImportTavernCharacterCardCreatesLoreItems(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)

	result, err := service.ImportTavernCharacterCard("liuyun.json", []byte(`{
		"spec": "chara_card_v2",
		"data": {
			"name": "柳云",
			"description": "负责整理情报",
			"character_book": {
				"entries": [
					{"keys": ["暗线"], "comment": "秘密", "content": "知道城主府暗线", "enabled": true}
				]
			}
		}
	}`))
	if err != nil {
		t.Fatalf("导入角色卡失败: %v", err)
	}
	if result.TargetPath != loreItemsFilePath || result.EntryCount != 1 || result.ItemCount != 2 {
		t.Fatalf("导入结果不符合预期: %#v", result)
	}

	items, err := NewLoreStore(workspace).List()
	if err != nil {
		t.Fatalf("读取资料库失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("资料库条目数不符合预期: %#v", items)
	}
	combined := items[0].Content + "\n" + items[1].Content
	for _, want := range []string{"负责整理情报", "知道城主府暗线"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("导入内容缺少 %q:\n%s", want, combined)
		}
	}
	if items[0].Type != "character" || items[0].Name != "柳云" {
		t.Fatalf("角色资料条目不符合预期: %#v", items[0])
	}
}

func TestParseProvidedTavernPNGReference(t *testing.T) {
	path := filepath.Join("..", "..", "import_一家之主_8542e9.png")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("本地未提供示例酒馆角色卡 PNG")
	}
	if err != nil {
		t.Fatalf("读取示例 PNG 失败: %v", err)
	}
	card, err := parseTavernCharacterCard(filepath.Base(path), data)
	if err != nil {
		t.Fatalf("解析示例 PNG 失败: %v", err)
	}
	if card.Name != "一家之主" {
		t.Fatalf("示例角色卡名称不符合预期: %q", card.Name)
	}
	if characterBookEntryCount(card.CharacterBook) == 0 {
		t.Fatalf("示例角色卡应包含世界书条目")
	}
}

func makeTestPNGTextChunk(keyword, text string) []byte {
	var data []byte
	data = append(data, pngSignature...)
	chunkData := append([]byte(keyword), 0)
	chunkData = append(chunkData, []byte(text)...)
	data = appendPNGChunk(data, "tEXt", chunkData)
	data = appendPNGChunk(data, "IEND", nil)
	return data
}

func appendPNGChunk(dst []byte, chunkType string, chunkData []byte) []byte {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(chunkData)))
	dst = append(dst, length[:]...)
	dst = append(dst, []byte(chunkType)...)
	dst = append(dst, chunkData...)
	dst = append(dst, 0, 0, 0, 0)
	return dst
}
