package api

import (
	"context"
	"denova/internal/book"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	restoreResp := performJSONRequest(t, server, http.MethodPost, "/api/versions/"+backupID+"/restore", map[string]any{
		"paths": []string{"chapters/ch01.md"},
	})
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
		Revision  string `json:"revision"`
		Workspace string `json:"workspace"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	if readBody.Revision == "" {
		t.Fatalf("读取文件应返回 revision")
	}
	if readBody.Workspace != application.Workspace() {
		t.Fatalf("读取文件应返回 canonical workspace: got=%q want=%q", readBody.Workspace, application.Workspace())
	}

	if err := application.BookService().WriteFile("chapters/ch01.md", "Agent 已更新的新内容"); err != nil {
		t.Fatalf("Agent 写入失败: %v", err)
	}

	writeResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/file", map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "前端旧内容",
		"base_revision": readBody.Revision,
		"workspace":     readBody.Workspace,
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

func TestWorkspaceFileWriteRejectsDifferentWorkspaceIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "当前内容"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	readResp := performJSONRequest(t, server, http.MethodGet, "/api/workspace/file?path=chapters%2Fch01.md", nil)
	var readBody struct {
		Revision string `json:"revision"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)

	writeResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/file", map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "不应写入",
		"base_revision": readBody.Revision,
		"workspace":     filepath.Join(t.TempDir(), "another-workspace"),
	})
	if writeResp.Code != http.StatusConflict {
		t.Fatalf("write status = %d body=%s", writeResp.Code, writeResp.Body.String())
	}
	var errorBody struct {
		Code string `json:"code"`
	}
	decodeResponse(t, writeResp.Body.Bytes(), &errorBody)
	if errorBody.Code != "workspace_changed" {
		t.Fatalf("error code = %q body=%s", errorBody.Code, writeResp.Body.String())
	}
	got, err := application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || got != "当前内容" {
		t.Fatalf("工作区身份冲突不得写文件: content=%q err=%v", got, err)
	}
}

func TestWorkspaceFileWriteReportsNoop(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "未变化"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	readResp := performJSONRequest(t, server, http.MethodGet, "/api/workspace/file?path=chapters%2Fch01.md", nil)
	var readBody struct {
		Revision  string `json:"revision"`
		Workspace string `json:"workspace"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	writeResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/file", map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "未变化",
		"base_revision": readBody.Revision,
		"workspace":     readBody.Workspace,
	})
	if writeResp.Code != http.StatusOK {
		t.Fatalf("write status = %d body=%s", writeResp.Code, writeResp.Body.String())
	}
	var writeBody struct {
		Workspace string `json:"workspace"`
		Changed   bool   `json:"changed"`
	}
	decodeResponse(t, writeResp.Body.Bytes(), &writeBody)
	if writeBody.Workspace != readBody.Workspace {
		t.Fatalf("保存响应 workspace=%q want=%q", writeBody.Workspace, readBody.Workspace)
	}
	if writeBody.Changed {
		t.Fatalf("同内容保存应报告 changed=false: %s", writeResp.Body.String())
	}
}

func TestVersionPathRestoreAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	ctx := context.Background()
	if err := application.BookService().Create("chapters/ch01.md", "file", "第一版"); err != nil {
		t.Fatalf("创建章节失败: %v", err)
	}
	first, err := application.CreateVersion(ctx, "初始版本")
	if err != nil || first.Version == nil {
		t.Fatalf("创建初始版本失败: %#v err=%v", first, err)
	}
	if err := application.BookService().WriteFile("chapters/ch01.md", "第二版"); err != nil {
		t.Fatalf("更新章节失败: %v", err)
	}

	// The version service suite covers modified, deleted, and added paths plus
	// HEAD preservation. This API test keeps one path to verify request/response
	// wiring without repeating the slower multi-commit repository scenario.
	body := map[string]any{"paths": []string{"chapters/ch01.md"}}
	planResp := performJSONRequest(t, server, http.MethodPost, "/api/versions/"+first.Version.ID+"/restore-plan", body)
	if planResp.Code != http.StatusOK {
		t.Fatalf("restore-plan status = %d body=%s", planResp.Code, planResp.Body.String())
	}
	var plan book.VersionRestorePlan
	decodeResponse(t, planResp.Body.Bytes(), &plan)
	if plan.Scope != book.VersionRestoreScopePaths || plan.WillCreateBackup || len(plan.Changes) != 1 {
		t.Fatalf("unexpected restore plan: %#v", plan)
	}

	restoreResp := performJSONRequest(t, server, http.MethodPost, "/api/versions/"+first.Version.ID+"/restore", body)
	if restoreResp.Code != http.StatusOK {
		t.Fatalf("restore status = %d body=%s", restoreResp.Code, restoreResp.Body.String())
	}
	var result book.VersionRestoreResult
	decodeResponse(t, restoreResp.Body.Bytes(), &result)
	if result.Scope != book.VersionRestoreScopePaths || result.BackupVersion != nil || len(result.RestoredPaths) != 1 {
		t.Fatalf("unexpected restore result: %#v", result)
	}
	if result.RestoredPaths[0] != "chapters/ch01.md" {
		t.Fatalf("unexpected restored path: %#v", result.RestoredPaths)
	}
	restored, err := application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || restored != "第一版" {
		t.Fatalf("restored chapter = %q err=%v", restored, err)
	}
}

func TestVersionWorkspaceRestorePlanAnnouncesBackupAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "第一版"); err != nil {
		t.Fatalf("创建章节失败: %v", err)
	}
	first, err := application.CreateVersion(context.Background(), "初始版本")
	if err != nil || first.Version == nil {
		t.Fatalf("创建初始版本失败: %#v err=%v", first, err)
	}
	if err := application.BookService().WriteFile("chapters/ch01.md", "第二版"); err != nil {
		t.Fatalf("更新章节失败: %v", err)
	}
	workspacePlanResp := performJSONRequest(t, server, http.MethodPost, "/api/versions/"+first.Version.ID+"/restore-plan", nil)
	if workspacePlanResp.Code != http.StatusOK {
		t.Fatalf("workspace restore-plan status = %d body=%s", workspacePlanResp.Code, workspacePlanResp.Body.String())
	}
	var workspacePlan book.VersionRestorePlan
	decodeResponse(t, workspacePlanResp.Body.Bytes(), &workspacePlan)
	if workspacePlan.Scope != book.VersionRestoreScopeWorkspace || !workspacePlan.WillCreateBackup || workspacePlan.BackupMessage == "" {
		t.Fatalf("dirty workspace rollback should announce backup: %#v", workspacePlan)
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

func TestWorkspaceReplaceLiteralAndRegex(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()
	if err := application.BookService().Create("chapters/ch01.md", "file", "林川和韩月进城。\n林川和韩月出城。\n"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := application.BookService().Create("notes.txt", "file", "ABC abc"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 字面量替换：大小写不敏感，命中同文件多处。
	literalResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query":       "abc",
		"replacement": "xyz",
		"regex":       false,
		"workspace":   workspace,
	})
	if literalResp.Code != http.StatusOK {
		t.Fatalf("literal replace status = %d body=%s", literalResp.Code, literalResp.Body.String())
	}
	var literalBody struct {
		TotalReplacements int `json:"total_replacements"`
		Files             []struct {
			Path         string `json:"path"`
			Replacements int    `json:"replacements"`
		} `json:"files"`
		Skipped []string `json:"skipped"`
	}
	decodeResponse(t, literalResp.Body.Bytes(), &literalBody)
	if literalBody.TotalReplacements != 2 || len(literalBody.Files) != 1 || literalBody.Files[0].Path != "notes.txt" || len(literalBody.Skipped) != 0 {
		t.Fatalf("字面量替换响应不符合预期: %s", literalResp.Body.String())
	}
	got, err := application.BookService().ReadFile("notes.txt")
	if err != nil || got != "xyz xyz" {
		t.Fatalf("字面量替换结果不符合预期: content=%q err=%v", got, err)
	}

	// 正则替换：捕获组引用（$2与$1 需按 JS 语义展开）。
	regexResp := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query":       `(林川)和(韩月)`,
		"replacement": "$2与$1",
		"regex":       true,
		"workspace":   workspace,
	})
	if regexResp.Code != http.StatusOK {
		t.Fatalf("regex replace status = %d body=%s", regexResp.Code, regexResp.Body.String())
	}
	var regexBody struct {
		TotalReplacements int `json:"total_replacements"`
	}
	decodeResponse(t, regexResp.Body.Bytes(), &regexBody)
	if regexBody.TotalReplacements != 2 {
		t.Fatalf("正则替换应替换两处，实际: %s", regexResp.Body.String())
	}
	got, err = application.BookService().ReadFile("chapters/ch01.md")
	if err != nil || got != "韩月与林川进城。\n韩月与林川出城。\n" {
		t.Fatalf("正则替换结果不符合预期: content=%q err=%v", got, err)
	}

	// 替换前应创建可恢复版本。
	history, err := application.VersionHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("读取版本历史失败: %v", err)
	}
	foundBackup := false
	for _, item := range history {
		if item.Message == "全局替换前自动备份" {
			foundBackup = true
			break
		}
	}
	if !foundBackup {
		t.Fatalf("替换前应创建可恢复版本，历史: %#v", history)
	}
}

func TestWorkspaceReplaceValidatesRequest(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()
	if err := application.BookService().Create("chapters/ch01.md", "file", "正文 abc"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	emptyQuery := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query": "", "replacement": "x", "workspace": workspace,
	})
	if emptyQuery.Code != http.StatusBadRequest {
		t.Fatalf("空 query 应返回 400，实际 = %d body=%s", emptyQuery.Code, emptyQuery.Body.String())
	}

	invalidRegex := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query": "(未闭合", "replacement": "x", "regex": true, "workspace": workspace,
	})
	if invalidRegex.Code != http.StatusBadRequest {
		t.Fatalf("非法正则应返回 400，实际 = %d body=%s", invalidRegex.Code, invalidRegex.Body.String())
	}

	emptyMatch := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query": "a*", "replacement": "x", "regex": true, "workspace": workspace,
	})
	if emptyMatch.Code != http.StatusBadRequest {
		t.Fatalf("可匹配空串的正则应返回 400，实际 = %d body=%s", emptyMatch.Code, emptyMatch.Body.String())
	}

	// 无匹配：200 且不替换、不创建备份版本。
	noMatch := performJSONRequest(t, server, http.MethodPost, "/api/workspace/replace", map[string]any{
		"query": "不存在的词", "replacement": "x", "workspace": workspace,
	})
	if noMatch.Code != http.StatusOK {
		t.Fatalf("无匹配替换应返回 200，实际 = %d body=%s", noMatch.Code, noMatch.Body.String())
	}
	var noMatchBody struct {
		TotalReplacements int `json:"total_replacements"`
	}
	decodeResponse(t, noMatch.Body.Bytes(), &noMatchBody)
	if noMatchBody.TotalReplacements != 0 {
		t.Fatalf("无匹配时不应替换，实际: %s", noMatch.Body.String())
	}
	history, err := application.VersionHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("读取版本历史失败: %v", err)
	}
	for _, item := range history {
		if item.Message == "全局替换前自动备份" {
			t.Fatalf("无匹配时不应创建备份版本，历史: %#v", history)
		}
	}
}
