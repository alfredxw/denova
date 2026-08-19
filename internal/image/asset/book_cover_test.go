package asset

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	imagegen "denova/internal/image/generation"
)

type coverFakeGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
}

func (g *coverFakeGenerator) Generate(_ context.Context, _ *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error) {
	g.request = request
	return g.result, nil
}

func TestGenerateWritesCoverSourceMetaAndBackup(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.WriteBinaryFile(CoverPath, []byte("old-cover")); err != nil {
		t.Fatalf("写入旧封面失败: %v", err)
	}
	generator := &coverFakeGenerator{result: imagegen.Result{
		ProfileID:    "cover-profile",
		Provider:     "openai",
		Model:        "gpt-image-1",
		Size:         "1728x2304",
		OutputFormat: "png",
		Images: []imagegen.Image{{
			Data:          []byte("new-cover"),
			MIMEType:      "image/png",
			Extension:     "png",
			RevisedPrompt: "revised",
		}},
	}}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.GenerateCover(context.Background(), &config.Config{}, bookService, CoverGenerateRequest{
		Title:             "星河边境",
		Description:       "舰队与边城。",
		Instruction:       "冷色调",
		ImagePresetID:     "realistic",
		ImagePresetPrompt: "真实光影",
	})
	if err != nil {
		t.Fatal(err)
	}
	if generator.request.Size != "1728x2304" || generator.request.OutputFormat != "png" || generator.request.N != 1 {
		t.Fatalf("封面生成参数不符合预期: %#v", generator.request)
	}
	for _, required := range []string{"真实光影", "星河边境", "舰队与边城", "冷色调"} {
		if !strings.Contains(generator.request.Prompt, required) {
			t.Fatalf("prompt 缺少 %q:\n%s", required, generator.request.Prompt)
		}
	}
	if result.CoverPath != CoverPath {
		t.Fatalf("展示封面路径不符合预期: %s", result.CoverPath)
	}
	if result.SourcePath != "assets/image/covers/20260628-120000-abcd1234/cover.png" {
		t.Fatalf("原图路径不符合预期: %s", result.SourcePath)
	}
	if result.MetaPath != "assets/image/covers/20260628-120000-abcd1234/meta.json" {
		t.Fatalf("元数据路径不符合预期: %s", result.MetaPath)
	}
	if result.BackupPath != "assets/image/covers/backups/20260628-120000-previous.png" {
		t.Fatalf("旧封面备份路径不符合预期: %s", result.BackupPath)
	}

	assertFileBytes(t, workspace, CoverPath, "new-cover")
	assertFileBytes(t, workspace, result.SourcePath, "new-cover")
	assertFileBytes(t, workspace, result.BackupPath, "old-cover")
	meta, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatalf("读取元数据失败: %v", err)
	}
	for _, required := range []string{`"schema": "book_cover.v1"`, `"cover_path": "assets/image/cover.png"`, `"backup_path": "assets/image/covers/backups/20260628-120000-previous.png"`} {
		if !strings.Contains(string(meta), required) {
			t.Fatalf("元数据缺少 %q:\n%s", required, string(meta))
		}
	}
}

func TestGenerateWithoutExistingCoverSkipsBackup(t *testing.T) {
	workspace := t.TempDir()
	generator := &coverFakeGenerator{result: imagegen.Result{
		ProfileID:    "default",
		Provider:     "openai",
		Model:        "gpt-image-1",
		OutputFormat: "png",
		Images:       []imagegen.Image{{Data: []byte("cover"), Extension: "png"}},
	}}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.GenerateCover(context.Background(), &config.Config{}, book.NewService(workspace), CoverGenerateRequest{Title: "无旧封面"})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath != "" {
		t.Fatalf("无旧封面时不应产生备份: %#v", result)
	}
	assertFileBytes(t, workspace, CoverPath, "cover")
}

func TestGenerateConvertsJPEGToCanonicalPNGCover(t *testing.T) {
	workspace := t.TempDir()
	jpegData := testJPEG(t)
	generator := &coverFakeGenerator{result: imagegen.Result{
		ProfileID:    "grok",
		Provider:     "xai",
		Model:        "grok-imagine-image-2.0",
		OutputFormat: "jpeg",
		Images:       []imagegen.Image{{Data: jpegData, MIMEType: "image/jpeg", Extension: "jpeg"}},
	}}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "jpeg0001" }

	result, err := service.GenerateCover(context.Background(), &config.Config{}, book.NewService(workspace), CoverGenerateRequest{Title: "JPEG cover"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourcePath != "assets/image/covers/20260628-120000-jpeg0001/cover.jpeg" {
		t.Fatalf("source path = %q", result.SourcePath)
	}
	assertFileBytesEqual(t, workspace, result.SourcePath, jpegData)
	assertPNGFile(t, workspace, CoverPath)
}

func TestUploadWritesCoverSourceMetaAndBackup(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.WriteBinaryFile(CoverPath, []byte("old-cover")); err != nil {
		t.Fatalf("写入旧封面失败: %v", err)
	}
	service := NewServiceWithGenerator(nil)
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "upload01" }

	result, err := service.UploadCover(bookService, CoverUploadRequest{
		Filename: "cover.png",
		Data:     testPNG(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CoverPath != CoverPath {
		t.Fatalf("展示封面路径不符合预期: %s", result.CoverPath)
	}
	if result.SourcePath != "assets/image/covers/20260628-120000-upload01/upload.png" {
		t.Fatalf("上传原图路径不符合预期: %s", result.SourcePath)
	}
	if result.MetaPath != "assets/image/covers/20260628-120000-upload01/meta.json" {
		t.Fatalf("元数据路径不符合预期: %s", result.MetaPath)
	}
	if result.BackupPath != "assets/image/covers/backups/20260628-120000-previous.png" {
		t.Fatalf("旧封面备份路径不符合预期: %s", result.BackupPath)
	}
	assertFileBytes(t, workspace, result.BackupPath, "old-cover")
	assertPNGFile(t, workspace, CoverPath)
	assertPNGFile(t, workspace, result.SourcePath)

	meta, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatalf("读取元数据失败: %v", err)
	}
	for _, required := range []string{`"source": "book_cover_upload"`, `"provider": "user_upload"`, `"cover_path": "assets/image/cover.png"`} {
		if !strings.Contains(string(meta), required) {
			t.Fatalf("元数据缺少 %q:\n%s", required, string(meta))
		}
	}
}

func assertFileBytes(t *testing.T, workspace, relPath, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", relPath, err)
	}
	if string(data) != want {
		t.Fatalf("%s 内容不符合预期: %q", relPath, string(data))
	}
}

func assertFileBytesEqual(t *testing.T, workspace, relPath string, want []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", relPath, err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("%s 内容不符合预期", relPath)
	}
}

func assertPNGFile(t *testing.T, workspace, relPath string) {
	t.Helper()
	file, err := os.Open(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("打开 %s 失败: %v", relPath, err)
	}
	defer file.Close()
	if _, err := png.Decode(file); err != nil {
		t.Fatalf("%s 不是有效 PNG: %v", relPath, err)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("生成测试 JPEG 失败: %v", err)
	}
	return buf.Bytes()
}
