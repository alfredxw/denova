package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// TestHandleUnknownTool 验证 LLM 幻觉调用不存在工具时，处理器返回引导性
// ToolMessage 而不是抛出错误，从而让 Agent 自行修正。
func TestHandleUnknownTool(t *testing.T) {
	result, err := handleUnknownTool(context.Background(), "write_todo", `{"todos":[]}`)
	if err != nil {
		t.Fatalf("处理未知工具不应返回错误: %v", err)
	}
	if !strings.Contains(result, "write_todo") {
		t.Fatalf("结果应包含工具名: %s", result)
	}
	if !strings.Contains(result, "[tool error]") {
		t.Fatalf("结果应携带 [tool error] 前缀以提示模型自我修复: %s", result)
	}
}

func TestInteractiveStoryToolMiddlewareBlocksWriteTools(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			called = true
			return "ok", nil
		},
		&adk.ToolContext{Name: "write_file"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"file_path":"/tmp/a"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("write_file should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "互动故事模式禁止使用写文件工具") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestInteractiveStoryToolMiddlewareAllowsReadTools(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			called = true
			return "ok", nil
		},
		&adk.ToolContext{Name: "read_file"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || result != "ok" {
		t.Fatalf("read_file should pass through, called=%v result=%s", called, result)
	}
}
