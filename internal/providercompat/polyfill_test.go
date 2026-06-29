package providercompat

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeChatModel is a minimal ToolCallingChatModel that returns a fixed
// message. We use it to assert that Wrap repairs the inner model's output.
type fakeChatModel struct {
	fixedMsg *schema.Message
	stream   *schema.StreamReader[*schema.Message]
}

func (f *fakeChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.fixedMsg, nil
}
func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.stream != nil {
		return f.stream, nil
	}
	if f.fixedMsg == nil {
		return schema.StreamReaderFromArray([]*schema.Message{}), nil
	}
	return schema.StreamReaderFromArray([]*schema.Message{f.fixedMsg}), nil
}
func (f *fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

// nonStandardProviderCfg 模拟一个走 OpenAI 兼容协议、但输出格式不标准的
// provider（可能是本地 LM、特定第三方、或者旧版本）。任何字段或输出
// 与 OpenAI 官方不完全一致的 provider 都会触发 polyfill。
var nonStandardProviderCfg = openai.ChatModelConfig{
	BaseURL: "https://example.invalid/v1/",
	Model:   "non-standard-model-v1",
}

func TestExtraRequestFields_NonStandardProvider(t *testing.T) {
	got := ExtraRequestFields(nonStandardProviderCfg)
	if v, ok := got["reasoning_split"]; !ok || v != true {
		t.Fatalf("expected reasoning_split=true for non-standard provider, got %v", got)
	}
}

func TestExtraRequestFields_OpenAIProvider(t *testing.T) {
	for _, cfg := range []openai.ChatModelConfig{
		{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"},
		{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
	} {
		if got := ExtraRequestFields(cfg); len(got) != 0 {
			t.Fatalf("expected no extras for %s, got %v", cfg.BaseURL, got)
		}
	}
}

func TestWrap_NonStandardProvider_RepairsToolCallAndThink(t *testing.T) {
	// 复刻一个返回非标准输出的模型：think + 文本工具调用 + 内部特殊 token
	content := "<think>Let me load the skill.</think>\n\n" +
		"加载 rewrite skill 的具体流程。<tool_call>\n" +
		"<invoke name=\"skill\"><skill>rewrite</skill></invoke>\n" +
		"</tool_call>"
	inner := &fakeChatModel{fixedMsg: &schema.Message{Role: schema.Assistant, Content: content}}
	wrapped := Wrap(inner, nonStandardProviderCfg)
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

func TestWrap_NonStandardProvider_PreservesNativeToolCalls(t *testing.T) {
	idx := 0
	inner := &fakeChatModel{fixedMsg: &schema.Message{
		Role:    schema.Assistant,
		Content: "正文",
		ToolCalls: []schema.ToolCall{{
			Index: &idx, ID: "x", Type: "function",
			Function: schema.FunctionCall{Name: "read_file", Arguments: "{}"},
		}},
	}}
	out, err := Wrap(inner, nonStandardProviderCfg).Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("native tool calls altered: %#v", out.ToolCalls)
	}
}

func TestWrap_OpenAIProvider_PassThrough(t *testing.T) {
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

func TestWrap_NonStandardProvider_StreamReturnsBeforeUpstreamEOF(t *testing.T) {
	upstream, writer := schema.Pipe[*schema.Message](1)
	inner := &fakeChatModel{stream: upstream}
	wrapped := Wrap(inner, nonStandardProviderCfg)

	writer.Send(&schema.Message{Role: schema.Assistant, Content: "第一段"}, nil)

	result := make(chan *schema.StreamReader[*schema.Message], 1)
	errs := make(chan error, 1)
	go func() {
		stream, err := wrapped.Stream(context.Background(), nil)
		if err != nil {
			errs <- err
			return
		}
		result <- stream
	}()

	var stream *schema.StreamReader[*schema.Message]
	select {
	case err := <-errs:
		t.Fatalf("stream returned error before first frame: %v", err)
	case stream = <-result:
	case <-time.After(100 * time.Millisecond):
		writer.Close()
		t.Fatal("wrapped stream blocked until upstream EOF instead of returning first frame")
	}
	defer stream.Close()

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv first frame: %v", err)
	}
	if got.Content != "第一段" {
		t.Fatalf("first frame content = %q", got.Content)
	}

	writer.Send(&schema.Message{Role: schema.Assistant, Content: "第二段"}, nil)
	writer.Close()
	got, err = stream.Recv()
	if err != nil {
		t.Fatalf("recv second frame: %v", err)
	}
	if got.Content != "第二段" {
		t.Fatalf("second frame content = %q", got.Content)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("final recv error = %v, want EOF", err)
	}
}

func TestWrap_NonStandardProvider_StreamParsesTextToolCallAfterStreamingPrelude(t *testing.T) {
	upstream := schema.StreamReaderFromArray([]*schema.Message{
		{Role: schema.Assistant, Content: "先说明"},
		{Role: schema.Assistant, Content: "<tool_"},
		{Role: schema.Assistant, Content: "call><invoke name=\"read_file\"><path>chapters/ch01.md</path></invoke></tool_call>"},
	})
	wrapped := Wrap(&fakeChatModel{stream: upstream}, nonStandardProviderCfg)
	stream, err := wrapped.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv prelude: %v", err)
	}
	if first.Content != "先说明" {
		t.Fatalf("prelude content = %q", first.Content)
	}
	toolFrame, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv tool frame: %v", err)
	}
	if len(toolFrame.ToolCalls) != 1 || toolFrame.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool frame not parsed: %#v", toolFrame.ToolCalls)
	}
	if toolFrame.ToolCalls[0].Function.Arguments != `{"path":"chapters/ch01.md"}` {
		t.Fatalf("tool args = %q", toolFrame.ToolCalls[0].Function.Arguments)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("final recv error = %v, want EOF", err)
	}
}

func TestWrap_NonStandardProvider_StreamStripsInlineThinkWithoutWaitingForEOF(t *testing.T) {
	upstream, writer := schema.Pipe[*schema.Message](1)
	wrapped := Wrap(&fakeChatModel{stream: upstream}, nonStandardProviderCfg)
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "<think>先想"}, nil)

	stream, err := wrapped.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv thinking frame: %v", err)
	}
	if got.ReasoningContent != "先想" || got.Content != "" {
		t.Fatalf("thinking frame = %#v", got)
	}
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "</think>\n正文"}, nil)
	writer.Close()

	got, err = stream.Recv()
	if err != nil {
		t.Fatalf("recv content frame: %v", err)
	}
	if got.Content != "正文" || got.ReasoningContent != "" {
		t.Fatalf("content frame = %#v", got)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("final recv error = %v, want EOF", err)
	}
}
