package compat

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

// fakeChatModel is a minimal ToolCallingChatModel that returns a fixed
// message. We use it to assert that Wrap repairs the inner model's output.
type fakeChatModel struct {
	fixedMsg *agent.Message
	stream   *agent.StreamReader[*agent.Message]
}

func (f *fakeChatModel) Generate(_ context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return f.fixedMsg, nil
}
func (f *fakeChatModel) Stream(_ context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	if f.stream != nil {
		return f.stream, nil
	}
	if f.fixedMsg == nil {
		return agent.StreamReaderFromArray([]*agent.Message{}), nil
	}
	return agent.StreamReaderFromArray([]*agent.Message{f.fixedMsg}), nil
}
func (f *fakeChatModel) WithTools(_ []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	return f, nil
}

// nonStandardProviderCfg describes a known endpoint dialect explicitly. The
// compatibility layer never guesses behavior from provider names or URLs.
var nonStandardProviderCfg = Config{
	Model:                 "non-standard-model-v1",
	RepairTextToolCalls:   true,
	RepairInlineThinking:  true,
	RequestReasoningSplit: true,
}

func TestExtraRequestFields_NonStandardProvider(t *testing.T) {
	got := ExtraRequestFields(nonStandardProviderCfg)
	if v, ok := got["reasoning_split"]; !ok || v != true {
		t.Fatalf("expected reasoning_split=true for non-standard provider, got %v", got)
	}
}

func TestExtraRequestFields_OmittedUnlessExplicit(t *testing.T) {
	if got := ExtraRequestFields(Config{Model: "any-model"}); len(got) != 0 {
		t.Fatalf("expected no implicit extras, got %v", got)
	}
}

func TestWrap_NonStandardProvider_RepairsToolCallAndThink(t *testing.T) {
	// 复刻一个返回非标准输出的模型：think + 文本工具调用 + 内部特殊 token
	content := "<think>Let me load the skill.</think>\n\n" +
		"加载 rewrite skill 的具体流程。<tool_call>\n" +
		"<invoke name=\"skill\"><skill>rewrite</skill></invoke>\n" +
		"</tool_call>"
	inner := &fakeChatModel{fixedMsg: &agent.Message{Role: agent.Assistant, Content: content}}
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

func TestTextToolCallIDsRemainUniqueAcrossResponses(t *testing.T) {
	const content = `<tool_call><invoke name="read"><path>chapter.md</path></invoke></tool_call>`
	seen := make(map[string]bool)
	for range 4 {
		message := &agent.Message{Role: agent.Assistant, Content: content}
		extractTextToolCalls(message)
		if len(message.ToolCalls) != 1 {
			t.Fatalf("tool calls = %#v", message.ToolCalls)
		}
		id := message.ToolCalls[0].ID
		if !strings.HasPrefix(id, "text_tool_call_") {
			t.Fatalf("synthetic tool call ID = %q", id)
		}
		if seen[id] {
			t.Fatalf("synthetic tool call ID repeated across responses: %q", id)
		}
		seen[id] = true
	}
}

func TestWrap_NonStandardProvider_PreservesNativeToolCalls(t *testing.T) {
	idx := 0
	inner := &fakeChatModel{fixedMsg: &agent.Message{
		Role:    agent.Assistant,
		Content: "正文",
		ToolCalls: []agent.ToolCall{{
			Index: &idx, ID: "x", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: "{}"},
		}},
	}}
	out, err := Wrap(inner, nonStandardProviderCfg).Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Function.Name != "read" {
		t.Fatalf("native tool calls altered: %#v", out.ToolCalls)
	}
}

func TestWrap_OpenAIProvider_PassThrough(t *testing.T) {
	inner := &fakeChatModel{fixedMsg: &agent.Message{Role: agent.Assistant, Content: "raw <think>oops</think> done"}}
	cfg := Config{Model: "gpt-4o"}
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
	upstream, writer := agent.Pipe[*agent.Message](1)
	inner := &fakeChatModel{stream: upstream}
	wrapped := Wrap(inner, nonStandardProviderCfg)

	writer.Send(&agent.Message{Role: agent.Assistant, Content: "第一段"}, nil)

	result := make(chan *agent.StreamReader[*agent.Message], 1)
	errs := make(chan error, 1)
	go func() {
		stream, err := wrapped.Stream(context.Background(), nil)
		if err != nil {
			errs <- err
			return
		}
		result <- stream
	}()

	var stream *agent.StreamReader[*agent.Message]
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

	writer.Send(&agent.Message{Role: agent.Assistant, Content: "第二段"}, nil)
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
	upstream := agent.StreamReaderFromArray([]*agent.Message{
		{Role: agent.Assistant, Content: "先说明"},
		{Role: agent.Assistant, Content: "<tool_"},
		{Role: agent.Assistant, Content: "call><invoke name=\"read\"><path>chapters/ch01.md</path></invoke></tool_call>"},
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
	if len(toolFrame.ToolCalls) != 1 || toolFrame.ToolCalls[0].Function.Name != "read" {
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
	upstream, writer := agent.Pipe[*agent.Message](1)
	wrapped := Wrap(&fakeChatModel{stream: upstream}, nonStandardProviderCfg)
	writer.Send(&agent.Message{Role: agent.Assistant, Content: "<think>先想"}, nil)

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
	writer.Send(&agent.Message{Role: agent.Assistant, Content: "</think>\n正文"}, nil)
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
