package providercompat

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeChatModel is a minimal ToolCallingChatModel that returns a fixed
// message. We use it to assert that Wrap repairs the inner model's output.
type fakeChatModel struct {
	fixedMsg *schema.Message
}

func (f *fakeChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.fixedMsg, nil
}
func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.fixedMsg == nil {
		return schema.StreamReaderFromArray([]*schema.Message{}), nil
	}
	return schema.StreamReaderFromArray([]*schema.Message{f.fixedMsg}), nil
}
func (f *fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

func TestExtraRequestFields_MiniMax(t *testing.T) {
	cfg := openai.ChatModelConfig{BaseURL: "https://minimaxi.com/v1/", Model: "MiniMax-M3"}
	got := ExtraRequestFields(cfg)
	if v, ok := got["reasoning_split"]; !ok || v != true {
		t.Fatalf("expected reasoning_split=true for MiniMax, got %v", got)
	}
}

func TestExtraRequestFields_OtherProvider(t *testing.T) {
	for _, cfg := range []openai.ChatModelConfig{
		{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"},
		{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
	} {
		if got := ExtraRequestFields(cfg); len(got) != 0 {
			t.Fatalf("expected no extras for %s, got %v", cfg.BaseURL, got)
		}
	}
}

func TestWrap_MiniMax_RepairsToolCallAndThink(t *testing.T) {
	// 复刻真实 MiniMax-M3 输出：think + 文本工具调用 + 内部特殊 token
	content := "<think>Let me load the skill.</think>\n\n" +
		"加载 rewrite skill 的具体流程。<tool_call>\n" +
		"<invoke name=\"skill\"><skill>rewrite</skill></invoke>\n" +
		"</tool_call>"
	inner := &fakeChatModel{fixedMsg: &schema.Message{Role: schema.Assistant, Content: content}}
	cfg := openai.ChatModelConfig{BaseURL: "https://minimaxi.com/v1/", Model: "MiniMax-M3"}

	wrapped := Wrap(inner, cfg)
	out, err := wrapped.Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d (%#v)", len(out.ToolCalls), out.ToolCalls)
	}
	if name := out.ToolCalls[0].Function.Name; name != "skill" {
		t.Fatalf("tool name = %q, want skill", name)
	}
	if args := out.ToolCalls[0].Function.Arguments; args != `{"skill":"rewrite"}` {
		t.Fatalf("args = %q, want {\"skill\":\"rewrite\"}", args)
	}
	if out.Content != "加载 rewrite skill 的具体流程。" {
		t.Fatalf("content = %q", out.Content)
	}
	if !strings.Contains(out.ReasoningContent, "load the skill") {
		t.Fatalf("reasoning not captured: %q", out.ReasoningContent)
	}
}

func TestWrap_MiniMax_PreservesNativeToolCalls(t *testing.T) {
	idx := 0
	inner := &fakeChatModel{fixedMsg: &schema.Message{
		Role:    schema.Assistant,
		Content: "正文",
		ToolCalls: []schema.ToolCall{{
			Index: &idx, ID: "x", Type: "function",
			Function: schema.FunctionCall{Name: "read_file", Arguments: "{}"},
		}},
	}}
	cfg := openai.ChatModelConfig{BaseURL: "https://minimaxi.com/v1/", Model: "MiniMax-M3"}
	out, err := Wrap(inner, cfg).Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("native tool calls altered: %#v", out.ToolCalls)
	}
}

func TestWrap_OtherProvider_PassThrough(t *testing.T) {
	inner := &fakeChatModel{fixedMsg: &schema.Message{Role: schema.Assistant, Content: "raw <think>oops</think> done"}}
	cfg := openai.ChatModelConfig{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
	// OpenAI 端点：原样返回，think 标签不应被剥离（信任它走 reasoning_content 字段）
	out, err := Wrap(inner, cfg).Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "raw <think>oops</think> done" {
		t.Fatalf("OpenAI output unexpectedly modified: %q", out.Content)
	}
	if out.ReasoningContent != "" {
		t.Fatalf("OpenAI reasoning unexpectedly populated: %q", out.ReasoningContent)
	}
}
