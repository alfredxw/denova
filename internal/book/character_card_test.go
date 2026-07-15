package book

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
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
	if result.TargetPath != loreItemsRelPath(workspace) || result.EntryCount != 1 || result.ItemCount != 2 {
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
	if strings.Contains(items[0].BriefDescription, "负责整理情报") || !strings.Contains(items[0].BriefDescription, "角色「柳云」") {
		t.Fatalf("导入简介应是检索提示，不应截取角色正文: %#v", items[0])
	}
}

func TestServiceImportTavernCharacterCardAddsNumericSuffixForDuplicateLoreNames(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	store := NewLoreStore(workspace)
	if _, err := store.Create(LoreItemInput{Type: "world", Name: "秘密", Importance: "important", Content: "既有资料"}); err != nil {
		t.Fatal(err)
	}

	result, err := service.ImportTavernCharacterCard("liuyun.json", []byte(`{
		"spec": "chara_card_v2",
		"data": {
			"name": "柳云",
			"description": "负责整理情报",
			"character_book": {
				"entries": [
					{"keys": ["暗线"], "comment": "秘密", "content": "知道城主府暗线", "enabled": true},
					{"keys": ["密道"], "comment": "秘密", "content": "知道城主府密道", "enabled": true}
				]
			}
		}
	}`))
	if err != nil {
		t.Fatalf("导入角色卡失败: %v", err)
	}
	if result.ItemCount != 3 {
		t.Fatalf("导入资料数量不符合预期: %#v", result)
	}

	items, err := store.ListAll()
	if err != nil {
		t.Fatalf("读取资料库失败: %v", err)
	}
	namesByID := make(map[string]string, len(items))
	for _, item := range items {
		namesByID[item.ID] = item.Name
	}
	for _, id := range []string{"柳云", "秘密", "秘密-2", "秘密-3"} {
		if _, ok := namesByID[id]; !ok {
			t.Fatalf("资料库缺少 ID %s: %#v", id, namesByID)
		}
	}
	if namesByID["秘密-2"] != "秘密-2" || namesByID["秘密-3"] != "秘密-3" {
		t.Fatalf("重名导入应使用数字后缀名称: %#v", namesByID)
	}
}

func TestServiceImportTavernCharacterCardImportsPNGCoverOpeningsAndUserPlaceholder(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	payload := base64.StdEncoding.EncodeToString([]byte(`{
		"spec": "chara_card_v2",
		"spec_version": "2.0",
		"data": {
			"name": "枫江月",
			"description": "清冷的生物女老师，会称呼 {{user}}。",
			"scenario": "高三生物实验室",
			"first_mes": "主开场：枫江月站在讲台前。",
			"alternate_greetings": ["备用开场一", "备用开场二"],
			"character_book": {
				"entries": [
					{"keys": ["实验室"], "comment": "场景", "content": "实验室里有显微镜", "enabled": true},
					{"keys": ["隐藏"], "comment": "禁用场景", "content": "这条暂不启用", "enabled": false}
				]
			},
			"extensions": {"depth_prompt": {"prompt": "仅酒馆运行时使用"}}
		},
		"avatar": "none",
		"talkativeness": 0.5
	}`))
	png := makeTestPNGTextChunk("chara", payload)

	result, err := service.ImportTavernCharacterCard("fengjiangyue.png", png, CharacterCardImportOptions{UserCharacterName: "韩澈"})
	if err != nil {
		t.Fatalf("导入 PNG 角色卡失败: %v", err)
	}
	if result.CoverPath != tavernCardCoverPath {
		t.Fatalf("封面路径不符合预期: %#v", result)
	}
	if result.OpeningPresetPath != interactiveOpeningPresetPath || result.OpeningPresetCount != 3 {
		t.Fatalf("开场预设导入结果不符合预期: %#v", result)
	}
	if !result.UserPlaceholderFound {
		t.Fatalf("应检测到 {{user}} 占位符: %#v", result)
	}
	if result.UserCharacterName != "韩澈" {
		t.Fatalf("用户角色名不符合预期: %#v", result)
	}
	cover, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(tavernCardCoverPath)))
	if err != nil {
		t.Fatalf("读取封面失败: %v", err)
	}
	if string(cover) != string(png) {
		t.Fatalf("封面 PNG 未按原始文件写入")
	}
	openingData, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(interactiveOpeningPresetPath)))
	if err != nil {
		t.Fatalf("读取开场预设失败: %v", err)
	}
	openingText := string(openingData)
	for _, want := range []string{"主开场：枫江月站在讲台前。", "备用开场一", "备用开场二"} {
		if !strings.Contains(openingText, want) {
			t.Fatalf("开场预设缺少 %q:\n%s", want, openingText)
		}
	}

	items, err := NewLoreStore(workspace).List()
	if err != nil {
		t.Fatalf("读取资料库失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("启用资料应包含角色、{{user}} 和启用世界书条目: %#v", items)
	}
	combined := items[0].Content + "\n" + items[1].Content + "\n" + items[2].Content
	if strings.Contains(combined, "主开场：枫江月站在讲台前。") || strings.Contains(combined, "备用开场一") {
		t.Fatalf("开场白不应写入资料库条目:\n%s", combined)
	}
	if !strings.Contains(combined, "韩澈") || !strings.Contains(combined, "实验室里有显微镜") {
		t.Fatalf("资料库缺少用户角色或世界书内容:\n%s", combined)
	}
	if strings.Contains(combined, "这条暂不启用") {
		t.Fatalf("禁用世界书条目不应进入模型可见资料列表:\n%s", combined)
	}
	allItems, err := NewLoreStore(workspace).ListAll()
	if err != nil {
		t.Fatalf("读取完整资料库失败: %v", err)
	}
	if len(allItems) != 4 {
		t.Fatalf("完整资料库应保留禁用世界书条目: %#v", allItems)
	}
	foundDisabled := false
	for _, item := range allItems {
		if strings.Contains(item.Content, "这条暂不启用") {
			foundDisabled = !item.Enabled
		}
	}
	if !foundDisabled {
		t.Fatalf("禁用世界书条目应以 enabled=false 保留: %#v", allItems)
	}
	if !hasCompatibilityField(result.Compatibility.Capabilities, "narrative_openings") ||
		!hasCompatibilityField(result.Compatibility.Capabilities, "disabled_lore") ||
		!hasCompatibilityField(result.Compatibility.DiscardedExtensions, "unknown") {
		t.Fatalf("兼容性报告不符合预期: %#v", result.Compatibility)
	}
}

func TestPreviewTavernCharacterCardReportsCompatibility(t *testing.T) {
	preview, err := PreviewTavernCharacterCard("card.json", []byte(`{
		"data": {
			"name": "谢眠",
			"first_mes": "开场",
			"alternate_greetings": ["备用"],
			"creator": "tester",
			"extensions": {"foo": "bar"},
			"character_book": {"entries": [{"comment": "关闭", "content": "暂不启用", "enabled": false}]}
		},
		"talkativeness": 0.7
	}`))
	if err != nil {
		t.Fatalf("预览角色卡失败: %v", err)
	}
	if preview.OpeningPresetCount != 2 || preview.WillImportCover {
		t.Fatalf("预览导入计划不符合预期: %#v", preview)
	}
	if !hasCompatibilityField(preview.Compatibility.Capabilities, "narrative_openings") ||
		!hasCompatibilityField(preview.Compatibility.Capabilities, "disabled_lore") ||
		!hasCompatibilityField(preview.Compatibility.DiscardedExtensions, "unknown") {
		t.Fatalf("预览兼容性报告不符合预期: %#v", preview.Compatibility)
	}
}

func TestTavernWorldbookIsNormalizedWithoutLoadingEngine(t *testing.T) {
	workspace := t.TempDir()
	result, err := NewService(workspace).ImportTavernCharacterCard("card.json", []byte(`{
		"data":{"name":"归舟","description":"一名旅者","character_book":{"entries":[
			{"id":7,"comment":"地点：旧港","keys":["旧港","港口"],"secondary_keys":["雨夜","旧港"],"content":"地点：旧港\n常年下雨。<UpdateVariable>{\"weather\":\"rain\"}</UpdateVariable>","constant":true,"selective":true,"enabled":true},
			{"id":8,"comment":"远方传闻","keys":["北境"],"content":"北境正在结冰。","constant":false,"enabled":false},
			{"id":9,"comment":"MVU变量初始化","content":"<UpdateVariable>{\"hp\":100}</UpdateVariable>","constant":true,"enabled":true}
		]}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ItemCount != 3 {
		t.Fatalf("应导入角色、常驻地点和禁用按需资料，纯运行条目应删除: %#v", result)
	}
	items, err := NewLoreStore(workspace).ListAll()
	if err != nil {
		t.Fatal(err)
	}
	var location, rumor LoreItem
	for _, item := range items {
		switch item.Name {
		case "地点：旧港":
			location = item
		case "远方传闻":
			rumor = item
		}
	}
	if location.Type != "location" || location.LoadMode != LoreLoadModeResident {
		t.Fatalf("constant 明确地点应成为常驻地点: %#v", location)
	}
	if strings.Join(location.Keywords, ",") != "旧港,港口,雨夜" || strings.Contains(location.Content, "UpdateVariable") {
		t.Fatalf("关键词应合并去重且运行块应被清洗: %#v", location)
	}
	if rumor.Enabled || rumor.LoadMode != LoreLoadModeAuto || rumor.Type != "other" || rumor.TypeSource != LoreTypeSourceHeuristic {
		t.Fatalf("无明确类型信号的禁用条目应保留为按需 other，等待后续整理: %#v", rumor)
	}
	if location.Provenance == nil || location.Provenance.SourceRecordID != "7" || location.Provenance.SourceHash == "" {
		t.Fatalf("应记录模型不可见来源: %#v", location.Provenance)
	}
	if strings.Contains(location.Content, "来源文件") || len([]rune(location.BriefDescription)) > 240 ||
		strings.Contains(location.BriefDescription, "常年下雨") || !strings.Contains(location.BriefDescription, "搜索关键词：旧港、港口、雨夜") {
		t.Fatalf("正文不应混入来源元数据且简介应有界: %#v", location)
	}
	store := NewLoreStore(workspace)
	modelContext, err := store.ProgressiveContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(modelContext, "card.json") || strings.Contains(modelContext, location.Provenance.SourceHash) {
		t.Fatalf("provenance 不得进入模型上下文:\n%s", modelContext)
	}
	searchIndex, err := store.LoreIndexMarkdown(LoreIndexOptions{Keywords: []string{"旧港"}, Match: LoreIndexMatchAny})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(searchIndex, "关键词: 旧港、港口、雨夜") {
		t.Fatalf("资料索引应直接展示搜索关键词:\n%s", searchIndex)
	}
}

func TestLoreClassificationHeuristicRecognizesCommonWorldbookNames(t *testing.T) {
	tests := map[string]string{
		"人物详情：沈凝":                 "character",
		"详细人物-罗衡":                 "character",
		"角色档案·戒律长老":               "character",
		"地点：旧港":                   "location",
		"宗门：青岚宗":                  "faction",
		"力量体系：灵脉":                 "rule",
		"法宝：照夜镜":                  "item",
		"世界观：黄昏纪元":                "world",
		"Character Profile: Iris": "character",
		"Location - Old Harbor":   "location",
		"远方传闻":                    "other",
	}
	for name, want := range tests {
		got := ClassifyLoreItemHeuristic(LoreClassificationInput{Name: name})
		if got.Type != want {
			t.Fatalf("name %q classified as %s, want %s (%#v)", name, got.Type, want, got)
		}
	}
}

func TestCharacterCardImportSemanticallyClassifiesOnlyUncertainEntries(t *testing.T) {
	workspace := t.TempDir()
	called := 0
	result, err := NewService(workspace).ImportTavernCharacterCard("semantic.json", []byte(`{
		"data":{"name":"归舟","description":"旅者","character_book":{"entries":[
			{"id":1,"comment":"地点：旧港","content":"旧港常年下雨。","enabled":true},
			{"id":2,"comment":"沈凝","keys":["见证者"],"content":"她负责见证公开比试。","enabled":true}
		]}}
	}`), CharacterCardImportOptions{
		ClassificationMode: LoreClassificationModeSemantic,
		ClassifyLore: func(inputs []LoreClassificationInput) ([]LoreClassificationSuggestion, error) {
			called++
			if len(inputs) != 1 || inputs[0].Name != "沈凝" || len([]byte(inputs[0].Content)) > semanticLoreClassificationBodyBytes {
				t.Fatalf("semantic classifier should receive only bounded uncertain entries: %#v", inputs)
			}
			return []LoreClassificationSuggestion{{ID: inputs[0].ID, Type: "character", Confidence: LoreClassificationConfidenceHigh, Reason: "正文描述人物职责"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 || result.ClassificationMode != LoreClassificationModeSemantic || result.UncertainTypeCount != 0 || result.ClassificationCounts["character"] != 1 || result.ClassificationCounts["location"] != 1 {
		t.Fatalf("unexpected semantic classification summary: called=%d result=%#v", called, result)
	}
	items, err := NewLoreStore(workspace).ListAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == "沈凝" && (item.Type != "character" || item.TypeSource != LoreTypeSourceSemantic) {
			t.Fatalf("semantic type provenance not persisted: %#v", item)
		}
		if item.Name == "地点：旧港" && item.TypeSource != LoreTypeSourceHeuristic {
			t.Fatalf("high-confidence local classification should not call the model: %#v", item)
		}
	}
}

func TestCharacterCardImportDoesNotPersistLowConfidenceSemanticSuggestion(t *testing.T) {
	workspace := t.TempDir()
	result, err := NewService(workspace).ImportTavernCharacterCard("low-confidence.json", []byte(`{
		"data":{"name":"归舟","description":"旅者","character_book":{"entries":[
			{"id":1,"comment":"沈凝","content":"也许是人物，也可能是一处代号。","enabled":true}
		]}}
	}`), CharacterCardImportOptions{
		ClassificationMode: LoreClassificationModeSemantic,
		ClassifyLore: func(inputs []LoreClassificationInput) ([]LoreClassificationSuggestion, error) {
			return []LoreClassificationSuggestion{{ID: inputs[0].ID, Type: "character", Confidence: LoreClassificationConfidenceLow, Reason: "证据不足"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UncertainTypeCount != 1 {
		t.Fatalf("low-confidence suggestion should remain uncertain: %#v", result)
	}
	items, err := NewLoreStore(workspace).ListAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == "沈凝" && (item.Type != "other" || item.TypeSource != LoreTypeSourceHeuristic) {
			t.Fatalf("low-confidence semantic suggestion must not overwrite the heuristic result: %#v", item)
		}
	}
}

func TestTavernOpeningSanitizationKeepsOnlyNarrativeGameText(t *testing.T) {
	card := normalizedTavernCard{
		Name:     "命定之诗",
		FirstMes: "【首页】",
		AlternateGreetings: []string{
			`<!doctype html><html><style>body{}</style><script>boot()</script></html>`,
			`<customized>请先选择职业</customized>`,
			`<gametxt>清晨，钟声穿过王城。骑士推开门。</gametxt><UpdateVariable>{"day":1}</UpdateVariable><StatusPlaceHolderImpl/>`,
			`<gametxt>海风吹过码头，少女望向远方。</gametxt><JSONPatch>[]</JSONPatch>`,
			`<gametxt>雪落在北境的旧路上。</gametxt><% state.hp = 10 %>`,
		},
	}
	presets := tavernCardOpeningPresets(card)
	if len(presets) != 3 {
		t.Fatalf("应只保留 3 个纯叙事开场: %#v", presets)
	}
	for _, preset := range presets {
		if strings.Contains(preset.Content, "UpdateVariable") || strings.Contains(preset.Content, "JSONPatch") || strings.Contains(preset.Content, "<%") {
			t.Fatalf("开场仍含运行时内容: %#v", preset)
		}
	}
}

func TestPNGPrefersCCV3AndWarnsOnConflict(t *testing.T) {
	png := append([]byte{}, pngSignature...)
	png = appendPNGChunk(png, "tEXt", append(append([]byte("chara"), 0), []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"旧数据"}`)))...))
	png = appendPNGChunk(png, "tEXt", append(append([]byte("ccv3"), 0), []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"新数据"}`)))...))
	png = appendPNGChunk(png, "IEND", nil)
	preview, err := PreviewTavernCharacterCard("conflict.png", png)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Name != "新数据" || len(preview.Compatibility.Warnings) == 0 {
		t.Fatalf("应优先 ccv3 并报告冲突: %#v", preview)
	}
}

func TestCharacterCardImportAboveRecommendationWarnsButSucceeds(t *testing.T) {
	workspace := t.TempDir()
	content := strings.Repeat("常驻设定", 5000)
	raw := []byte(`{"data":{"name":"预算测试","description":"角色","character_book":{"entries":[{"id":1,"comment":"规则：长设定","content":` + strconv.Quote(content) + `,"constant":true,"enabled":true}]}}}`)
	preview, err := PreviewTavernCharacterCard("budget.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.ResidentLoreWarning || preview.ResidentLoreWarningKB != 32 {
		t.Fatalf("超过建议值时应只提示: %#v", preview)
	}
	result, err := NewService(workspace).ImportTavernCharacterCard("budget.json", raw)
	if err != nil {
		t.Fatalf("超过建议值不得阻止导入: %v", err)
	}
	if result.ItemCount == 0 {
		t.Fatalf("应写入资料: %#v", result)
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

func hasCompatibilityField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
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
