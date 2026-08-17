package api

import (
	"context"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentBookProjectDeleteCreatesRestorableVersion(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "正文"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	deleteResp := performJSONRequest(t, server, http.MethodPost, "/api/projects/"+url.PathEscape(application.ProjectID())+"/files/operations", map[string]any{
		"operations": []map[string]string{{"kind": "delete", "path": "chapters/ch01.md"}},
	})
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

	restoreResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/versions/"+backupID+"/restore"), map[string]any{
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

func TestProjectFileWriteRejectsStaleRevision(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "前端旧内容"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	readResp := performJSONRequest(t, server, http.MethodGet, projectWorkspaceAPI(application, "/files/file?path=chapters%2Fch01.md"), nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResp.Code, readResp.Body.String())
	}
	var readBody struct {
		Revision  string `json:"revision"`
		ProjectID string `json:"project_id"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	if readBody.Revision == "" {
		t.Fatalf("读取文件应返回 revision")
	}
	if readBody.ProjectID != application.ProjectID() {
		t.Fatalf("读取文件应返回 Project identity: got=%q want=%q", readBody.ProjectID, application.ProjectID())
	}

	if err := application.BookService().WriteFile("chapters/ch01.md", "Agent 已更新的新内容"); err != nil {
		t.Fatalf("Agent 写入失败: %v", err)
	}

	writeResp := performJSONRequest(t, server, http.MethodPut, projectWorkspaceAPI(application, "/files/file"), map[string]string{
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

func TestProjectFileWriteUsesRouteIdentityWhenAnotherBookIsForeground(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	workspace := application.Workspace()
	if err := application.BookService().Create("chapters/ch01.md", "file", "当前内容"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	readResp := performJSONRequest(t, server, http.MethodGet, projectWorkspaceAPI(application, "/files/file?path=chapters%2Fch01.md"), nil)
	var readBody struct {
		Revision string `json:"revision"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)

	created := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "Foreground File Book"})
	if created.Code != http.StatusOK || application.ProjectID() == projectID {
		t.Fatalf("create foreground Book status=%d project_id=%q body=%s", created.Code, application.ProjectID(), created.Body.String())
	}
	writeResp := performJSONRequest(t, server, http.MethodPut, "/api/projects/"+url.PathEscape(projectID)+"/files/file", map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "后台 Project 内容",
		"base_revision": readBody.Revision,
	})
	if writeResp.Code != http.StatusOK {
		t.Fatalf("write status = %d body=%s", writeResp.Code, writeResp.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "chapters", "ch01.md"))
	if err != nil || string(data) != "后台 Project 内容" {
		t.Fatalf("background Project file content=%q err=%v", data, err)
	}
	if _, err := application.BookService().ReadFile("chapters/ch01.md"); !os.IsNotExist(err) {
		t.Fatalf("foreground Book must remain isolated, err=%v", err)
	}
}

type routeIdentityWorkspaceFixture struct {
	application         *runtimeapp.App
	server              *Server
	backgroundProject   string
	backgroundWorkspace string
	foregroundProject   string
}

func newRouteIdentityWorkspaceFixture(t *testing.T) routeIdentityWorkspaceFixture {
	t.Helper()
	application := newTestApplication(t)
	server := NewServer(application, "0")
	backgroundProjectID := application.ProjectID()
	backgroundWorkspace := application.Workspace()
	if err := application.BookService().Create("notes.txt", "file", "alpha marker"); err != nil {
		t.Fatal(err)
	}

	created := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "Foreground Search Book"})
	if created.Code != http.StatusOK || application.ProjectID() == backgroundProjectID {
		t.Fatalf("create foreground Book status=%d project_id=%q body=%s", created.Code, application.ProjectID(), created.Body.String())
	}
	return routeIdentityWorkspaceFixture{
		application: application, server: server,
		backgroundProject: backgroundProjectID, backgroundWorkspace: backgroundWorkspace,
		foregroundProject: application.ProjectID(),
	}
}

func TestProjectSearchReplaceUsesRouteIdentityWhenAnotherBookIsForeground(t *testing.T) {
	fixture := newRouteIdentityWorkspaceFixture(t)
	backgroundAPI := "/api/projects/" + url.PathEscape(fixture.backgroundProject)

	search := performJSONRequest(t, fixture.server, http.MethodGet, backgroundAPI+"/workspace/search?q=alpha", nil)
	if search.Code != http.StatusOK {
		t.Fatalf("background search status=%d body=%s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Results []book.SearchResult `json:"results"`
	}
	decodeResponse(t, search.Body.Bytes(), &searchBody)
	if len(searchBody.Results) != 1 || searchBody.Results[0].Path != "notes.txt" {
		t.Fatalf("background search escaped its Project: %#v", searchBody.Results)
	}

	replace := performJSONRequest(t, fixture.server, http.MethodPost, backgroundAPI+"/workspace/replace", map[string]any{
		"query": "alpha", "replacement": "beta",
	})
	if replace.Code != http.StatusOK {
		t.Fatalf("background replace status=%d body=%s", replace.Code, replace.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(fixture.backgroundWorkspace, "notes.txt"))
	if err != nil || string(content) != "beta marker" {
		t.Fatalf("background replacement content=%q err=%v", content, err)
	}
	if _, err := fixture.application.BookService().ReadFile("notes.txt"); !os.IsNotExist(err) {
		t.Fatalf("foreground Book received background replacement, err=%v", err)
	}
}

func TestProjectVersionsUseRouteIdentityWhenAnotherBookIsForeground(t *testing.T) {
	fixture := newRouteIdentityWorkspaceFixture(t)
	backgroundAPI := "/api/projects/" + url.PathEscape(fixture.backgroundProject)
	checkpoint := performJSONRequest(t, fixture.server, http.MethodPost, backgroundAPI+"/versions", map[string]string{
		"message": "Background checkpoint",
	})
	if checkpoint.Code != http.StatusOK {
		t.Fatalf("background version status=%d body=%s", checkpoint.Code, checkpoint.Body.String())
	}

	backgroundHistory := performJSONRequest(t, fixture.server, http.MethodGet, backgroundAPI+"/versions?limit=10", nil)
	foregroundHistory := performJSONRequest(t, fixture.server, http.MethodGet, "/api/projects/"+url.PathEscape(fixture.foregroundProject)+"/versions?limit=10", nil)
	if backgroundHistory.Code != http.StatusOK {
		t.Fatalf("background history status=%d body=%s", backgroundHistory.Code, backgroundHistory.Body.String())
	}
	if foregroundHistory.Code != http.StatusOK {
		t.Fatalf("foreground history status=%d body=%s", foregroundHistory.Code, foregroundHistory.Body.String())
	}
	var backgroundBody, foregroundBody struct {
		Versions []book.VersionEntry `json:"versions"`
	}
	decodeResponse(t, backgroundHistory.Body.Bytes(), &backgroundBody)
	decodeResponse(t, foregroundHistory.Body.Bytes(), &foregroundBody)
	if !versionHistoryContainsMessage(backgroundBody.Versions, "Background checkpoint") {
		t.Fatalf("background checkpoint missing from Project history: %#v", backgroundBody.Versions)
	}
	if versionHistoryContainsMessage(foregroundBody.Versions, "Background checkpoint") {
		t.Fatalf("background checkpoint leaked into foreground history: %#v", foregroundBody.Versions)
	}
}

func TestProjectFileWriteReportsNoop(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.BookService().Create("chapters/ch01.md", "file", "未变化"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	readResp := performJSONRequest(t, server, http.MethodGet, projectWorkspaceAPI(application, "/files/file?path=chapters%2Fch01.md"), nil)
	var readBody struct {
		Revision  string `json:"revision"`
		ProjectID string `json:"project_id"`
	}
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	writeResp := performJSONRequest(t, server, http.MethodPut, projectWorkspaceAPI(application, "/files/file"), map[string]string{
		"path":          "chapters/ch01.md",
		"content":       "未变化",
		"base_revision": readBody.Revision,
	})
	if writeResp.Code != http.StatusOK {
		t.Fatalf("write status = %d body=%s", writeResp.Code, writeResp.Body.String())
	}
	var writeBody struct {
		ProjectID string `json:"project_id"`
		Changed   bool   `json:"changed"`
	}
	decodeResponse(t, writeResp.Body.Bytes(), &writeBody)
	if writeBody.ProjectID != readBody.ProjectID {
		t.Fatalf("保存响应 project_id=%q want=%q", writeBody.ProjectID, readBody.ProjectID)
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
	planResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/versions/"+first.Version.ID+"/restore-plan"), body)
	if planResp.Code != http.StatusOK {
		t.Fatalf("restore-plan status = %d body=%s", planResp.Code, planResp.Body.String())
	}
	var plan book.VersionRestorePlan
	decodeResponse(t, planResp.Body.Bytes(), &plan)
	if plan.Scope != book.VersionRestoreScopePaths || plan.WillCreateBackup || len(plan.Changes) != 1 {
		t.Fatalf("unexpected restore plan: %#v", plan)
	}

	restoreResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/versions/"+first.Version.ID+"/restore"), body)
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
	workspacePlanResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/versions/"+first.Version.ID+"/restore-plan"), nil)
	if workspacePlanResp.Code != http.StatusOK {
		t.Fatalf("workspace restore-plan status = %d body=%s", workspacePlanResp.Code, workspacePlanResp.Body.String())
	}
	var workspacePlan book.VersionRestorePlan
	decodeResponse(t, workspacePlanResp.Body.Bytes(), &workspacePlan)
	if workspacePlan.Scope != book.VersionRestoreScopeWorkspace || !workspacePlan.WillCreateBackup || workspacePlan.BackupMessage == "" {
		t.Fatalf("dirty workspace rollback should announce backup: %#v", workspacePlan)
	}
}

func TestProjectFileAssetServesProjectImages(t *testing.T) {
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

	okResp := performJSONRequest(t, server, http.MethodGet, projectWorkspaceAPI(application, "/files/asset?path=assets%2Fillustrations%2Fch01%2Fimage.png"), nil)
	if okResp.Code != http.StatusOK {
		t.Fatalf("asset status = %d body=%s", okResp.Code, okResp.Body.String())
	}
	if got := string(okResp.Body.Bytes()); got != string([]byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("asset body = %q", got)
	}
	if contentType := string(okResp.Header().Peek("Content-Type")); !strings.HasPrefix(contentType, "image/png") {
		t.Fatalf("content type = %q", contentType)
	}
	nonAssetResp := performJSONRequest(t, server, http.MethodGet, projectWorkspaceAPI(application, "/files/asset?path=chapters%2Fnot-asset.png"), nil)
	if nonAssetResp.Code != http.StatusOK {
		t.Fatalf("non-asset image status = %d body=%s", nonAssetResp.Code, nonAssetResp.Body.String())
	}

	for _, path := range []string{
		projectWorkspaceAPI(application, "/files/asset?path=assets%2Fillustrations%2F..%2F..%2Fchapters%2Fnot-asset.png"),
		projectWorkspaceAPI(application, "/files/asset?path=assets%2Fillustrations%2Fch01%2Fmeta.json"),
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
	if err := application.BookService().Create("chapters/ch01.md", "file", "林川和韩月进城。\n林川和韩月出城。\n"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := application.BookService().Create("notes.txt", "file", "ABC abc"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 字面量替换：大小写不敏感，命中同文件多处。
	literalResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query":       "abc",
		"replacement": "xyz",
		"regex":       false,
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
	regexResp := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query":       `(林川)和(韩月)`,
		"replacement": "$2与$1",
		"regex":       true,
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
		if strings.Contains(item.Message, "全局替换前自动备份") {
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
	if err := application.BookService().Create("chapters/ch01.md", "file", "正文 abc"); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	emptyQuery := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query": "", "replacement": "x",
	})
	if emptyQuery.Code != http.StatusBadRequest {
		t.Fatalf("空 query 应返回 400，实际 = %d body=%s", emptyQuery.Code, emptyQuery.Body.String())
	}

	invalidRegex := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query": "(未闭合", "replacement": "x", "regex": true,
	})
	if invalidRegex.Code != http.StatusBadRequest {
		t.Fatalf("非法正则应返回 400，实际 = %d body=%s", invalidRegex.Code, invalidRegex.Body.String())
	}

	emptyMatch := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query": "a*", "replacement": "x", "regex": true,
	})
	if emptyMatch.Code != http.StatusBadRequest {
		t.Fatalf("可匹配空串的正则应返回 400，实际 = %d body=%s", emptyMatch.Code, emptyMatch.Body.String())
	}

	// 无匹配：200 且不替换、不创建备份版本。
	noMatch := performJSONRequest(t, server, http.MethodPost, projectWorkspaceAPI(application, "/workspace/replace"), map[string]any{
		"query": "不存在的词", "replacement": "x",
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
		if strings.Contains(item.Message, "全局替换前自动备份") {
			t.Fatalf("无匹配时不应创建备份版本，历史: %#v", history)
		}
	}
}

func projectWorkspaceAPI(application interface{ ProjectID() string }, suffix string) string {
	return "/api/projects/" + url.PathEscape(application.ProjectID()) + suffix
}

func versionHistoryContainsMessage(history []book.VersionEntry, message string) bool {
	for _, entry := range history {
		if entry.Message == message {
			return true
		}
	}
	return false
}
