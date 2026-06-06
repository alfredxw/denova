package agent

import (
	"context"
	"strings"
	"testing"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"

	"nova/config"
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

func TestNewFilesystemMiddlewareRespectsToolSettings(t *testing.T) {
	backend, err := localbk.NewBackend(context.Background(), &localbk.Config{})
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := newFilesystemMiddleware(context.Background(), backend, config.ResolvedAgentToolSettings{
		FileRead:     true,
		FileWrite:    false,
		ShellExecute: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if middleware == nil {
		t.Fatal("filesystem middleware should be registered when read tools are enabled")
	}
	_, runCtx, err := middleware.BeforeAgent(context.Background(), &adk.ChatModelAgentContext{})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range runCtx.Tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"ls", "read_file", "glob", "grep"} {
		if !names[name] {
			t.Fatalf("read tool %s should be registered, names=%v", name, names)
		}
	}
	for _, name := range []string{"write_file", "edit_file", "execute"} {
		if names[name] {
			t.Fatalf("tool %s should be disabled, names=%v", name, names)
		}
	}
}
