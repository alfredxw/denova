package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/internal/book"
	"denova/internal/bookcover"
)

func TestCharacterCardImportAsNewBookAboveRecommendation(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	card, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"name":        "五十 KB 角色卡",
			"description": "角色设定",
			"character_book": map[string]any{"entries": []any{map[string]any{
				"id": 1, "comment": "规则：长设定", "content": strings.Repeat("常驻设定", 5000), "constant": true, "enabled": true,
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := characterCardImportBody(t, card, map[string]string{"target_mode": "new_book", "book_title": "五十 KB 新书"})
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/workspace/import-character-card",
		&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body.String())
	}
	var result struct {
		Workspace string `json:"workspace"`
		ItemCount int    `json:"item_count"`
	}
	decodeResponse(t, resp.Body.Bytes(), &result)
	if result.Workspace == "" || result.ItemCount == 0 {
		t.Fatalf("new-book import result mismatch: %#v", result)
	}
	residentBytes, err := book.NewLoreStore(result.Workspace).ResidentContentBytes()
	if err != nil {
		t.Fatal(err)
	}
	if residentBytes <= book.ResidentLoreWarningBytes {
		t.Fatalf("fixture should exceed warning recommendation: %d", residentBytes)
	}
}

func TestBookCoverUploadAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	body, contentType := bookCoverUploadBody(t, application.Workspace(), bookCoverAPITestPNG(t))
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/books/cover/upload",
		&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", resp.Code, resp.Body.String())
	}

	var result bookcover.Result
	decodeResponse(t, resp.Body.Bytes(), &result)
	if result.CoverPath != bookcover.CoverPath || result.CoverUpdatedAt == "" {
		t.Fatalf("上传封面响应不符合预期: %#v", result)
	}
	file, err := os.Open(filepath.Join(application.Workspace(), filepath.FromSlash(bookcover.CoverPath)))
	if err != nil {
		t.Fatalf("读取展示封面失败: %v", err)
	}
	defer file.Close()
	if _, err := png.Decode(file); err != nil {
		t.Fatalf("展示封面不是有效 PNG: %v", err)
	}
}

func TestBookExportTextAPI(t *testing.T) {
	application := newTestApplication(t)
	if _, err := application.UpdateBookInfo(application.Workspace(), "星河边境", "Denova", ""); err != nil {
		t.Fatalf("写入书籍元信息失败: %v", err)
	}
	if err := application.BookService().Create("chapters/ch00002-第二章-追光.md", "file", "第二章 追光\n\n林川踏入雨夜。"); err != nil {
		t.Fatalf("创建第二章失败: %v", err)
	}
	if err := application.BookService().Create("chapters/ch00001-第一章-开局.md", "file", "# 第一章 开局\n\n天亮了。"); err != nil {
		t.Fatalf("创建第一章失败: %v", err)
	}
	if err := application.BookService().Create("chapters/ch00003-空章.md", "file", ""); err != nil {
		t.Fatalf("创建空章失败: %v", err)
	}
	server := NewServer(application, "0")

	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/books/export?path="+url.QueryEscape(application.Workspace())+"&format=txt",
		nil,
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("export status = %d body=%s", resp.Code, resp.Body.String())
	}
	if contentType := string(resp.Header().Peek("Content-Type")); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
	disposition := string(resp.Header().Peek("Content-Disposition"))
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "filename*=UTF-8''%E6%98%9F%E6%B2%B3%E8%BE%B9%E5%A2%83.txt") {
		t.Fatalf("content disposition = %q", disposition)
	}
	body := resp.Body.String()
	for _, want := range []string{"星河边境", "作者: Denova", "第一章 开局", "天亮了。", "第二章 追光", "林川踏入雨夜。"} {
		if !strings.Contains(body, want) {
			t.Fatalf("export missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "# 第一章 开局") || strings.Contains(body, "空章") {
		t.Fatalf("export should remove duplicate heading and skip empty chapter:\n%s", body)
	}
}

func TestBookExportRejectsUnsupportedFormat(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/books/export?path="+url.QueryEscape(application.Workspace())+"&format=epub",
		nil,
	)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("export status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func bookCoverUploadBody(t *testing.T, path string, data []byte) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("path", path); err != nil {
		t.Fatalf("写入 path 字段失败: %v", err)
	}
	part, err := writer.CreateFormFile("file", "cover.png")
	if err != nil {
		t.Fatalf("创建文件字段失败: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入文件字段失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func characterCardImportBody(t *testing.T, data []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "card.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func bookCoverAPITestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	return buf.Bytes()
}
