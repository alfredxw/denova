package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/internal/book"
	"denova/internal/taskcenter"
)

func TestNovelImportAppearsInTaskCenterAsImportExport(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	result := performNovelImport(t, server, "测试书", "novel.txt")

	tasksResp := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if tasksResp.Code != http.StatusOK {
		t.Fatalf("task center status = %d body=%s", tasksResp.Code, tasksResp.Body.String())
	}
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResp.Body.Bytes(), &tasks)
	var importTask *taskcenter.Task
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Type == taskcenter.TaskTypeImportExport {
			importTask = &tasks.Tasks[i]
			break
		}
	}
	if importTask == nil {
		t.Fatalf("task center missing import/export task: %#v", tasks.Tasks)
	}
	if importTask.Status != taskcenter.StatusCompleted {
		t.Fatalf("import task status = %q, want %q", importTask.Status, taskcenter.StatusCompleted)
	}
	if importTask.Project.Path != result.Workspace {
		t.Fatalf("import task project path = %q, want %q", importTask.Project.Path, result.Workspace)
	}
	if importTask.Recovery.Kind != taskcenter.RecoveryKind("import_export") ||
		importTask.Recovery.Workspace != result.Workspace {
		t.Fatalf("import task recovery = %#v", importTask.Recovery)
	}
	if importTask.Recovery.TaskID == "" {
		t.Fatalf("completed import task is missing its execution task ID: %#v", importTask)
	}
	if want := "import-export:" + importTask.Recovery.TaskID; importTask.ID != want {
		t.Fatalf("completed import task ID = %q, want stable execution ID %q", importTask.ID, want)
	}
	if importTask.Error != "" {
		t.Fatalf("completed import task should not carry an error: %#v", importTask)
	}
}

func TestConsecutiveNovelImportsRemainDistinctInTaskCenter(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	first := performNovelImport(t, server, "第一本书", "first.txt")
	second := performNovelImport(t, server, "第二本书", "second.txt")

	tasksResp := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if tasksResp.Code != http.StatusOK {
		t.Fatalf("task center status = %d body=%s", tasksResp.Code, tasksResp.Body.String())
	}
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResp.Body.Bytes(), &tasks)
	var importTasks []taskcenter.Task
	for _, task := range tasks.Tasks {
		if task.Type == taskcenter.TaskTypeImportExport {
			importTasks = append(importTasks, task)
		}
	}
	if len(importTasks) != 2 {
		t.Fatalf("import/export task count = %d, want 2: %#v", len(importTasks), importTasks)
	}
	importsByWorkspace := make(map[string]taskcenter.Task)
	for _, task := range importTasks {
		importsByWorkspace[task.Project.Path] = task
	}
	firstTask, firstFound := importsByWorkspace[first.Workspace]
	secondTask, secondFound := importsByWorkspace[second.Workspace]
	if !firstFound || !secondFound {
		t.Fatalf("task center did not retain both imported books: %#v", importsByWorkspace)
	}
	if firstTask.ID == secondTask.ID {
		t.Fatalf("consecutive imports share task ID %q", firstTask.ID)
	}
	for _, task := range []taskcenter.Task{firstTask, secondTask} {
		if task.Status != taskcenter.StatusCompleted || task.Recovery.TaskID == "" {
			t.Fatalf("completed import task is not recoverable: %#v", task)
		}
		if want := "import-export:" + task.Recovery.TaskID; task.ID != want {
			t.Fatalf("completed import task ID = %q, want %q", task.ID, want)
		}
	}
}

func TestFailedNovelImportRetainsSourceWorkspaceRecovery(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	sourceWorkspace := application.Workspace()
	response := performNovelImportRequest(t, server, "失败导入", "unsupported.pdf")
	if response.Code != http.StatusOK {
		t.Fatalf("import status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "event: error") {
		t.Fatalf("import stream is missing the error event:\n%s", response.Body.String())
	}

	tasksResp := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if tasksResp.Code != http.StatusOK {
		t.Fatalf("task center status = %d body=%s", tasksResp.Code, tasksResp.Body.String())
	}
	var tasks taskcenter.ListResult
	decodeResponse(t, tasksResp.Body.Bytes(), &tasks)
	var failedTask *taskcenter.Task
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Type == taskcenter.TaskTypeImportExport && tasks.Tasks[i].Status == taskcenter.StatusFailed {
			failedTask = &tasks.Tasks[i]
			break
		}
	}
	if failedTask == nil {
		t.Fatalf("task center missing failed import/export task: %#v", tasks.Tasks)
	}
	if failedTask.Project.Path != sourceWorkspace {
		t.Fatalf("failed import project path = %q, want source workspace %q", failedTask.Project.Path, sourceWorkspace)
	}
	if failedTask.Recovery.Workspace != sourceWorkspace {
		t.Fatalf("failed import recovery workspace = %q, want source workspace %q", failedTask.Recovery.Workspace, sourceWorkspace)
	}
	if failedTask.Recovery.TaskID == "" {
		t.Fatalf("failed import task is missing its execution task ID: %#v", failedTask)
	}
	if want := "import-export:" + failedTask.Recovery.TaskID; failedTask.ID != want {
		t.Fatalf("failed import task ID = %q, want stable execution ID %q", failedTask.ID, want)
	}
	if failedTask.Error == "" {
		t.Fatalf("failed import task is missing its error: %#v", failedTask)
	}
}

func performNovelImport(t *testing.T, server *Server, title, filename string) book.NovelImportResult {
	t.Helper()
	response := performNovelImportRequest(t, server, title, filename)
	if response.Code != http.StatusOK {
		t.Fatalf("import status = %d body=%s", response.Code, response.Body.String())
	}
	var result book.NovelImportResult
	if !decodeNovelImportDoneEvent(t, response.Body.String(), &result) {
		t.Fatalf("import stream is missing the done event:\n%s", response.Body.String())
	}
	if result.Workspace == "" || result.ChapterCount != 2 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	return result
}

func performNovelImportRequest(t *testing.T, server *Server, title, filename string) *ut.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("book_title", title); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("split_strategy", "builtin"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("第一章 测试\n内容一行\n第二章 测试\n内容二行\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/books/import-novel",
		&ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()},
		ut.Header{Key: "Content-Type", Value: writer.FormDataContentType()},
	)
}

func decodeNovelImportDoneEvent(t *testing.T, body string, target *book.NovelImportResult) bool {
	t.Helper()
	for _, block := range strings.Split(body, "\n\n") {
		if !strings.Contains(block, "event: done") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), target); err != nil {
				t.Fatalf("decode done event failed: %v", err)
			}
			return true
		}
	}
	return false
}
