package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceDeleteCreatesRestorableVersion(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "正文"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	deleteResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/delete", map[string]string{"path": "chapters/ch01.md"})
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	deletedPath := filepath.Join(application.BookService().Workspace(), "chapters", "ch01.md")
	if _, err := os.Stat(deletedPath); !os.IsNotExist(err) {
		t.Fatalf("删除后文件应不存在，实际错误: %v", err)
	}

	history, err := application.VersionHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("读取版本历史失败: %v", err)
	}
	var backupID string
	for _, item := range history {
		if item.Message == "删除前自动备份" {
			backupID = item.ID
			break
		}
	}
	if backupID == "" {
		t.Fatalf("删除前应创建可恢复版本，历史: %#v", history)
	}

	restoreResp := performJSONRequest(t, server, http.MethodPost, "/api/versions/"+backupID+"/restore", nil)
	if restoreResp.Code != http.StatusOK {
		t.Fatalf("restore status = %d body=%s", restoreResp.Code, restoreResp.Body.String())
	}
	data, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("恢复后应能读取文件: %v", err)
	}
	if string(data) != "正文" {
		t.Fatalf("恢复内容不符合预期: %q", string(data))
	}
}

func TestWorkspaceFileWriteRejectsStaleRevision(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "前端旧内容"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	readResp := performJSONRequest(t, server, http.MethodGet, "/api/workspace/file?path=chapters%2Fch01.md", nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResp.Code, readResp.Body.String())
	}
	var readBody struct {
		Revision string `json:"revision"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	if readBody.Revision == "" {
		t.Fatalf("读取文件应返回 revision")
	}

	time.Sleep(2 * time.Millisecond)
	if err := application.BookService().WriteFile("chapters/ch01.md", "Agent 已更新的新内容"); err != nil {
		t.Fatalf("Agent 写入失败: %v", err)
	}

	writeResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/file", map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "前端旧内容",
		"base_revision": readBody.Revision,
	})
	if writeResp.Code != http.StatusConflict {
		t.Fatalf("write status = %d body=%s", writeResp.Code, writeResp.Body.String())
	}
	got, err := application.BookService().ReadFile("chapters/ch01.md")
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if got != "Agent 已更新的新内容" {
		t.Fatalf("冲突后应保留 Agent 内容，实际: %q", got)
	}
}

func TestWorkspaceAssetServesWorkspaceImages(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().WriteBinaryFile("assets/illustrations/ch01/image.png", []byte{0x89, 0x50, 0x4e, 0x47}); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := application.BookService().WriteFile("assets/illustrations/ch01/meta.json", "{}"); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if err := application.BookService().WriteBinaryFile("chapters/not-asset.png", []byte("png")); err != nil {
		t.Fatalf("write non asset image: %v", err)
	}

	okResp := performJSONRequest(t, server, http.MethodGet, "/api/workspace/asset?path=assets%2Fillustrations%2Fch01%2Fimage.png", nil)
	if okResp.Code != http.StatusOK {
		t.Fatalf("asset status = %d body=%s", okResp.Code, okResp.Body.String())
	}
	if got := string(okResp.Body.Bytes()); got != string([]byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("asset body = %q", got)
	}
	if contentType := string(okResp.Header().Peek("Content-Type")); !strings.HasPrefix(contentType, "image/png") {
		t.Fatalf("content type = %q", contentType)
	}
	nonAssetResp := performJSONRequest(t, server, http.MethodGet, "/api/workspace/asset?path=chapters%2Fnot-asset.png", nil)
	if nonAssetResp.Code != http.StatusOK {
		t.Fatalf("non-asset image status = %d body=%s", nonAssetResp.Code, nonAssetResp.Body.String())
	}

	for _, path := range []string{
		"/api/workspace/asset?path=assets%2Fillustrations%2F..%2F..%2Fchapters%2Fnot-asset.png",
		"/api/workspace/asset?path=assets%2Fillustrations%2Fch01%2Fmeta.json",
	} {
		resp := performJSONRequest(t, server, http.MethodGet, path, nil)
		if resp.Code == http.StatusOK {
			t.Fatalf("%s should be rejected", path)
		}
	}
}
