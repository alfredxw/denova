package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

type unavailableUpdateTransport struct{}

func (unavailableUpdateTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("private upstream diagnostics")
}

func TestUpdateFailuresUseRequestLocale(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = unavailableUpdateTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	handlers := New(nil)
	for _, test := range []struct {
		locale  string
		want    string
		install string
	}{
		{"zh-CN", "检查更新失败，请稍后重试。详细原因请查看服务端日志。", "安装更新失败，请重试或从 GitHub Releases 手动下载安装包。详细原因请查看服务端日志。"},
		{"en-US", "Could not check for updates. Try again later; see server logs for details.", "Could not install the update. Retry or download the archive from GitHub Releases; see server logs for details."},
	} {
		t.Run(test.locale, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.Set("X-Denova-Locale", test.locale)
			handlers.HandleUpdateCheck(context.Background(), request)
			var body map[string]string
			if err := json.Unmarshal(request.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if request.Response.StatusCode() != 502 || len(body) != 1 || body["error"] != test.want {
				t.Fatalf("response = %d %s", request.Response.StatusCode(), request.Response.Body())
			}
			request.Response.Reset()
			handlers.HandleUpdateInstall(context.Background(), request)
			if err := json.Unmarshal(request.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != test.install {
				t.Fatalf("install response = %s", request.Response.Body())
			}
			task := handlers.app.StartInstallUpdateTask(test.locale)
			select {
			case <-task.Done():
			case <-time.After(time.Second):
				t.Fatal("update task did not finish")
			}
			events, subscription := task.Subscribe()
			defer task.Unsubscribe(subscription)
			found := false
			for _, event := range events {
				if event.Event.Type != "error" {
					continue
				}
				data, ok := event.Event.Data.(map[string]string)
				if !ok || data["message"] != test.install {
					t.Fatalf("stream error = %#v", event)
				}
				found = true
			}
			if !found {
				t.Fatal("update task did not emit an error")
			}
		})
	}
}
