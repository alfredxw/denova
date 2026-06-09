package book

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitWorkspaceDoesNotCreateCharacterStates(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)

	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace 失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(state.SettingDir(), CharacterStatesFileName)); !os.IsNotExist(err) {
		t.Fatalf("InitWorkspace 不应自动创建 %s: %v", CharacterStatesFileName, err)
	}
}

func TestInitWorkspaceCreatesBrainstormMarkdown(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)

	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace 失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, BrainstormFileName)); err != nil {
		t.Fatalf("InitWorkspace 应创建 %s: %v", BrainstormFileName, err)
	}
}

func TestCompactContextIncludesCharacterStates(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), "outline.md"), []byte("大纲内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), "progress.md"), []byte("进度内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), CharacterStatesFileName), []byte("林川在废城东区地下仓库"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "ch0001-开局.md"), []byte("第一章正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.ChapterGroupDir(), "group01-废城.md"), []byte("章节组内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLoreStore(dir).Create(LoreItemInput{
		ID:         "hero",
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		LoadMode:   LoreLoadModeResident,
		Content:    "林川的长期人设",
	}); err != nil {
		t.Fatalf("创建资料失败: %v", err)
	}

	context := state.CompactContext()
	for _, required := range []string{
		"## 当前大纲",
		"大纲内容",
		"## 当前进度",
		"进度内容",
		"## 角色状态",
		"林川在废城东区地下仓库",
		"## 章节目录概览",
		"chapters/ch0001-开局.md",
		"## 常驻资料库",
		"林川的长期人设",
		"## 章节组细纲",
		"章节组内容",
	} {
		if !strings.Contains(context, required) {
			t.Fatalf("CompactContext 缺少 %q:\n%s", required, context)
		}
	}
}

func TestHasStateRecognizesCharacterStates(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := os.MkdirAll(state.SettingDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewLoreStore(dir).Ensure(); err != nil {
		t.Fatal(err)
	}
	if state.HasState() {
		t.Fatal("空作品不应有状态")
	}

	if err := os.WriteFile(filepath.Join(state.SettingDir(), CharacterStatesFileName), []byte("# 角色状态"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !state.HasState() {
		t.Fatal("只有 character-states.md 时也应识别为已有状态")
	}
}

func TestReadWriteBookMeta(t *testing.T) {
	// 测试 1：book.json 不存在时返回默认值
	t.Run("default_when_missing", func(t *testing.T) {
		dir := t.TempDir()
		s := NewState(dir)
		meta := s.ReadBookMeta()
		if meta.Title != filepath.Base(dir) {
			t.Errorf("期望 Title=%q, 实际=%q", filepath.Base(dir), meta.Title)
		}
		if meta.Author != "" || meta.Description != "" {
			t.Errorf("期望 Author 和 Description 为空")
		}
	})

	// 测试 2：写入后能正确读回
	t.Run("write_then_read", func(t *testing.T) {
		dir := t.TempDir()
		s := NewState(dir)
		input := BookMeta{
			Title:       "测试小说",
			Author:      "作者A",
			Description: "一段描述",
		}
		if err := s.WriteBookMeta(input); err != nil {
			t.Fatalf("WriteBookMeta 失败: %v", err)
		}
		got := s.ReadBookMeta()
		if got.Title != "测试小说" || got.Author != "作者A" || got.Description != "一段描述" {
			t.Errorf("读回内容不匹配: %+v", got)
		}
	})

	// 测试 3：写入自动设置 CreatedAt 和 UpdatedAt
	t.Run("auto_timestamps", func(t *testing.T) {
		dir := t.TempDir()
		s := NewState(dir)
		before := time.Now().Add(-time.Second)
		if err := s.WriteBookMeta(BookMeta{Title: "时间测试"}); err != nil {
			t.Fatalf("WriteBookMeta 失败: %v", err)
		}
		got := s.ReadBookMeta()
		createdAt, err := time.Parse(time.RFC3339, got.CreatedAt)
		if err != nil {
			t.Fatalf("解析 CreatedAt 失败: %v", err)
		}
		updatedAt, err := time.Parse(time.RFC3339, got.UpdatedAt)
		if err != nil {
			t.Fatalf("解析 UpdatedAt 失败: %v", err)
		}
		if createdAt.Before(before) || updatedAt.Before(before) {
			t.Error("时间戳早于写入前")
		}
	})

	// 测试 4：再次写入只更新 UpdatedAt，保留 CreatedAt
	t.Run("preserve_created_at", func(t *testing.T) {
		dir := t.TempDir()
		s := NewState(dir)

		// 手动写入一个带过去时间戳的 book.json
		oldCreated := "2024-01-01T00:00:00Z"
		oldUpdated := "2024-06-01T00:00:00Z"
		initial := BookMeta{Title: "第一版", CreatedAt: oldCreated, UpdatedAt: oldUpdated}
		data, _ := json.MarshalIndent(initial, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "book.json"), data, 0o644); err != nil {
			t.Fatalf("写入初始文件失败: %v", err)
		}

		// 带上原有 CreatedAt 再次写入
		if err := s.WriteBookMeta(BookMeta{
			Title:     "第二版",
			CreatedAt: oldCreated,
		}); err != nil {
			t.Fatalf("第二次写入失败: %v", err)
		}
		second := s.ReadBookMeta()
		if second.CreatedAt != oldCreated {
			t.Errorf("CreatedAt 被修改: 期望=%q, 实际=%q", oldCreated, second.CreatedAt)
		}
		if second.UpdatedAt == oldUpdated {
			t.Error("UpdatedAt 应该被更新")
		}
		if second.Title != "第二版" {
			t.Errorf("Title 应该更新为第二版, 实际=%q", second.Title)
		}
	})
}

func TestReadBookMetaFromDir(t *testing.T) {
	// 测试 1：目录不存在返回默认值
	t.Run("nonexistent_dir", func(t *testing.T) {
		meta := ReadBookMetaFromDir("/tmp/nova-test-nonexistent-dir-12345")
		if meta.Title != "nova-test-nonexistent-dir-12345" {
			t.Errorf("期望 Title 为目录名, 实际=%q", meta.Title)
		}
	})

	// 测试 2：有 book.json 时正确读取
	t.Run("read_existing", func(t *testing.T) {
		dir := t.TempDir()
		bm := BookMeta{
			Title:       "测试书",
			Author:      "测试作者",
			Description: "描述",
			CreatedAt:   "2025-01-01T00:00:00Z",
			UpdatedAt:   "2025-06-01T00:00:00Z",
		}
		data, _ := json.MarshalIndent(bm, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "book.json"), data, 0o644); err != nil {
			t.Fatalf("写入测试文件失败: %v", err)
		}
		got := ReadBookMetaFromDir(dir)
		if got.Title != "测试书" || got.Author != "测试作者" {
			t.Errorf("读取不匹配: %+v", got)
		}
		if got.CreatedAt != "2025-01-01T00:00:00Z" {
			t.Errorf("CreatedAt 不匹配: %q", got.CreatedAt)
		}
	})
}
