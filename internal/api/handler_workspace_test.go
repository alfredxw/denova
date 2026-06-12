package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
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
