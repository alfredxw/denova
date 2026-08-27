package handlers

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// 注入 handler 是薄壳,真实端到端 (HTTP → service → store) 由 app 包的
// service 测试覆盖。这里只钉路由注册、payload 解析与错误响应格式:
// 这些是 handler 包内可以不依赖真实 App 就能验证的全部内容。

func performMemoryAppendRequest(t *testing.T, body string) *ut.ResponseRecorder {
	t.Helper()
	server := hertzserver.Default()
	server.POST("/api/interactive/stories/:id/memory", New(nil).HandleInteractiveMemoryAppend)
	return ut.PerformRequest(server.Engine, http.MethodPost,
		"/api/interactive/stories/story-1/memory",
		&ut.Body{Body: bytes.NewReader([]byte(body)),Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
}

func TestHandleInteractiveMemoryAppendRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty body", body: "", want: "请求体为空"},
		{name: "malformed json", body: "{not json", want: "解析请求体失败"},
		{name: "missing source_turn_id", body: `{"records":[{"kind":"beat","subject":"甲","text":"x","evidence":"y"}]}`, want: "source_turn_id"},
		{name: "empty records", body: `{"source_turn_id":"t1","records":[]}`, want: "records 不能为空"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := performMemoryAppendRequest(t, tc.body)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 body=%s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), tc.want) {
				t.Fatalf("body must mention %q, got %s", tc.want, resp.Body.String())
			}
		})
	}
}

// TestHandleInteractiveMemoryAppendRouteRegistration 路由层是独立可测的小段:
// 必须挂上 POST 且路径参数 :id 可解析。其它四个错误响应已被上面的表驱动覆盖。
func TestHandleInteractiveMemoryAppendRouteRegistration(t *testing.T) {
	server := hertzserver.Default()
	server.POST("/api/interactive/stories/:id/memory", New(nil).HandleInteractiveMemoryAppend)

	// :id 必须出现在匹配路径里 —— 没有匹配的请求路径说明路由没挂上。
	resp := performMemoryAppendRequest(t, "")
	if resp.Code == http.StatusNotFound {
		t.Fatal("POST /api/interactive/stories/:id/memory must be registered")
	}
}