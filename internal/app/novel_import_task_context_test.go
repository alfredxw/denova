package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"denova/config"
	"denova/internal/book"
)

func TestNovelImportTaskRebindsSplitInferenceToTaskContext(t *testing.T) {
	modelCalled := make(chan struct{}, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case modelCalled <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		const jsonPayload = `{"split_regex":"^第[一二三四五六七八九十百千万零〇0-9]+[章节回集卷部]","reason":"ok"}`
		_, _ = w.Write([]byte(fmt.Sprintf(`data: {"choices":[{"index":0,"delta":{"content":%q},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n", jsonPayload)))
	}))
	t.Cleanup(modelServer.Close)

	dataDir := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey:        "test-key",
		OpenAIBaseURL:       modelServer.URL + "/v1",
		OpenAIModel:         "test-model",
		NovaDir:             dataDir,
		Workspace:           dataDir,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	task, err := application.StartNovelImportTask(context.Background(), NovelImportTaskRequest{
		Filename: "novel.txt",
		Data:     []byte("第一章 测试\n内容一行\n第二章 测试\n内容二行\n"),
		Title:    "后台导入",
		Options: book.NovelImportOptions{
			SplitStrategy: book.NovelImportSplitStrategyAgent,
			InferSplitRegex: func(string) (string, error) {
				return "", requestCtx.Err()
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(task.Abort)

	task.Wait()
	if task.Status() != TaskDone {
		t.Fatalf("import task status = %s, want done (error=%q)", task.Status(), task.Snapshot().Error)
	}
	select {
	case <-modelCalled:
	default:
		t.Fatal("split inference did not run under the task context; import fell back to builtin")
	}
}
