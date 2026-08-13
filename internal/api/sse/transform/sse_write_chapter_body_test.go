package transform

import (
	agentrun "denova/internal/agents/run"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSSEWriteChapterBodyMiddlewareShowsOnlyPathForToolCall(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	args := `{"path":"chapters/ch01.md","content":"第一行\n第二行"}`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
	}})

	gotArgs := eventDataString(got.Data, "args")
	if gotArgs != `{"path":"chapters/ch01.md"}` {
		t.Fatalf("display args should keep only path, got %q", gotArgs)
	}
	if strings.Contains(gotArgs, "第一行") || strings.Contains(gotArgs, "content") || strings.Contains(gotArgs, "...") {
		t.Fatalf("display args should not include body or placeholder, got %q", gotArgs)
	}
	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, fileCharCount("第一行\n第二行"))
}

func TestSSEWriteChapterBodyMiddlewareShowsOnlyPathForAbsoluteNovaChapterToolCall(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	path := `/Users/writer/.workspace/worktrees/999d/nova/.nova/测试/chapters/v00001-第一卷-废材逆袭/ch00001-第1章-陨落.md`
	args := `{"path":"` + path + `","content":"第一行\n第二行"}`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
	}})

	gotArgs := eventDataString(got.Data, "args")
	want := `{"path":"` + path + `"}`
	if gotArgs != want {
		t.Fatalf("display args should keep only absolute Nova chapter path, got %q", gotArgs)
	}
	if strings.Contains(gotArgs, "第一行") || strings.Contains(gotArgs, "content") || strings.Contains(gotArgs, "...") {
		t.Fatalf("display args should not include body or placeholder, got %q", gotArgs)
	}
	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, fileCharCount("第一行\n第二行"))
}

func TestSSEWriteChapterBodyMiddlewareShowsOnlyPathForPastedDetailArgs(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	path := `/Users/writer/.workspace/worktrees/999d/nova/.nova/测试/chapters/v00001-第一卷-废材逆袭/ch00011-第11章-水乳交融.md`
	args := `"path": "` + path + `", "content": "第一行\n第二行"`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
	}})

	gotArgs := eventDataString(got.Data, "args")
	want := `{"path":"` + path + `"}`
	if gotArgs != want {
		t.Fatalf("display args should keep only pasted absolute Nova chapter path, got %q", gotArgs)
	}
	if strings.Contains(gotArgs, "第一行") || strings.Contains(gotArgs, "content") || strings.Contains(gotArgs, "...") {
		t.Fatalf("display args should not include body or placeholder, got %q", gotArgs)
	}
	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, fileCharCount("第一行\n第二行"))
}

func TestSSEWriteChapterBodyMiddlewareUsesTargetWhenArgsCannotRevealPath(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	path := `/Users/writer/.workspace/worktrees/999d/nova/.nova/测试/chapters/v00001/ch00001.md`
	args := `{"content":"第一行\n第二行`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
		"target":     path,
	}})

	gotArgs := eventDataString(got.Data, "args")
	want := `{"path":"` + path + `"}`
	if gotArgs != want {
		t.Fatalf("display args should use target path when args cannot reveal path, got %q", gotArgs)
	}
	if strings.Contains(gotArgs, "第一行") || strings.Contains(gotArgs, "content") || strings.Contains(gotArgs, "...") {
		t.Fatalf("display args should not include body or placeholder, got %q", gotArgs)
	}
	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, fileCharCount("第一行\n第二行"))
}

func TestSSEWriteChapterBodyMiddlewareHoldsUnknownToolCallArgs(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       `{"content":"第一行`,
	}})

	if gotArgs := eventDataString(got.Data, "args"); gotArgs != "" {
		t.Fatalf("unknown write args should be held from SSE, got %q", gotArgs)
	}
}

func TestSSEWriteChapterBodyMiddlewareProjectsToolTargetToArgsDelta(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	path := `/Users/writer/.workspace/worktrees/999d/nova/.nova/测试/chapters/v00001/ch00001.md`
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_target", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"target":     path,
	}})
	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"content":"第一行`,
	}})

	if got.Type != "tool_args_delta" {
		t.Fatalf("tool_target should be projected to tool_args_delta, got %q", got.Type)
	}
	gotDelta := eventDataString(got.Data, "delta")
	want := `{"path":"` + path + `"}`
	if gotDelta != want {
		t.Fatalf("projected target delta = %q, want %q", gotDelta, want)
	}
	assertChapterBodyHiddenNotice(t, got.Data)
	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 2)
	progress := events[0]
	if progress.Type != "tool_args_delta" {
		t.Fatalf("flush progress event type = %q, want tool_args_delta", progress.Type)
	}
	assertChapterBodyHiddenNotice(t, progress.Data)
	assertGeneratedChars(t, progress.Data, fileCharCount("第一行"))
}

func TestSSEWriteChapterBodyMiddlewareDropsChapterContentDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	first := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"chapters/ch02.md","content":"第一行`,
	}})
	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `\n第二行\n第三行"}`,
	}})

	firstDelta := eventDataString(first.Data, "delta")
	if firstDelta != `{"path":"chapters/ch02.md"}` {
		t.Fatalf("first display delta should include only path: %q", firstDelta)
	}
	if strings.Contains(firstDelta, "第一行") || strings.Contains(firstDelta, "content") || strings.Contains(firstDelta, "...") {
		t.Fatalf("first display delta should not include body or placeholder: %q", firstDelta)
	}
	assertChapterBodyHiddenNotice(t, first.Data)
	assertGeneratedChars(t, first.Data, fileCharCount("第一行"))
	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 2)
	progress := events[0]
	if progressDelta := eventDataString(progress.Data, "delta"); progressDelta != "" {
		t.Fatalf("content progress delta should not include body: %q", progressDelta)
	}
	assertChapterBodyHiddenNotice(t, progress.Data)
	assertGeneratedChars(t, progress.Data, fileCharCount("第一行\n第二行\n第三行"))
}

func TestSSEWriteChapterBodyMiddlewareThrottlesGeneratedCharacterProgress(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	firstContent := "第一行"
	first := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"chapters/ch02.md","content":"` + firstContent,
	}})
	assertGeneratedChars(t, first.Data, fileCharCount(firstContent))

	heldContent := strings.Repeat("中", chapterBodyProgressStep-1)
	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      heldContent,
	}})

	progress := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      "文",
	}})
	if progressDelta := eventDataString(progress.Data, "delta"); progressDelta != "" {
		t.Fatalf("content progress delta should not include body: %q", progressDelta)
	}
	assertChapterBodyHiddenNotice(t, progress.Data)
	assertGeneratedChars(t, progress.Data, fileCharCount(firstContent+heldContent+"文"))

	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 1)
	if events[0].Type != "tool_result" {
		t.Fatalf("tool_result should be forwarded without extra flush, got %q", events[0].Type)
	}
}

func TestSSEWriteChapterBodyMiddlewareCountsFileCharacters(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	first := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"chapters/ch02.md","content":"第一行，abc123`,
	}})
	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `\n第二行。"}`,
	}})

	assertChapterBodyHiddenNotice(t, first.Data)
	assertGeneratedChars(t, first.Data, fileCharCount("第一行，abc123"))
	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 2)
	progress := events[0]
	if progressDelta := eventDataString(progress.Data, "delta"); progressDelta != "" {
		t.Fatalf("content progress delta should not include body: %q", progressDelta)
	}
	assertChapterBodyHiddenNotice(t, progress.Data)
	assertGeneratedChars(t, progress.Data, fileCharCount("第一行，abc123\n第二行。"))
}

func TestSSEWriteChapterBodyMiddlewareCountsJSONEscapesLikeWrittenFile(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	args := `{"path":"chapters/ch01.md","content":"\u4e2d\u6587\nA"}`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
	}})

	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, fileCharCount("中文\nA"))
}

func TestSSEWriteChapterBodyMiddlewareCorrectsFinalCountWithDecodedArgs(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	args := `{"path":"chapters/ch01.md","content":"开头\ud83d\ude00结尾"}`

	got := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       args,
	}})

	assertChapterBodyHiddenNotice(t, got.Data)
	assertGeneratedChars(t, got.Data, 6)
	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 2)
	assertGeneratedChars(t, events[0].Data, fileCharCount("开头😀结尾"))
}

func TestSSEWriteChapterBodyMiddlewareDropsDraftContentDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	first := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"drafts/ch02.md","content":"第一行`,
	}})

	if got := eventDataString(first.Data, "delta"); got != `{"path":"drafts/ch02.md"}` {
		t.Fatalf("draft display delta should include only path: %q", got)
	}
	assertChapterBodyHiddenNotice(t, first.Data)
	assertGeneratedChars(t, first.Data, fileCharCount("第一行"))
}

func TestSSEWriteChapterBodyMiddlewareDropsAbsoluteNovaChapterContentDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	first := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"/Users/writer/.workspace/worktrees/999d/nova/.nova/测试/chapters/v00001-第一卷-废材逆袭/ch00001-第1章-陨落.md","content":"第一行`,
	}})
	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `\n第二行"}`,
	}})

	firstDelta := eventDataString(first.Data, "delta")
	if !strings.Contains(firstDelta, `.nova/测试/chapters/`) {
		t.Fatalf("absolute Nova chapter delta should include path: %q", firstDelta)
	}
	if strings.Contains(firstDelta, "第一行") || strings.Contains(firstDelta, "content") || strings.Contains(firstDelta, "...") {
		t.Fatalf("absolute Nova chapter delta should not include body or placeholder: %q", firstDelta)
	}
	assertChapterBodyHiddenNotice(t, first.Data)
	assertGeneratedChars(t, first.Data, fileCharCount("第一行"))
	events := mustForwardSSEEvents(t, collector, handler, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
	}}, 2)
	progress := events[0]
	if progressDelta := eventDataString(progress.Data, "delta"); progressDelta != "" {
		t.Fatalf("absolute Nova content progress delta should not include body: %q", progressDelta)
	}
	assertChapterBodyHiddenNotice(t, progress.Data)
	assertGeneratedChars(t, progress.Data, fileCharCount("第一行\n第二行"))
}

func TestSSEWriteChapterBodyMiddlewareRestoresNonChapterDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	mustSuppressSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"set`,
	}})
	next := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `ting/outline.md","content":"第一行`,
	}})

	got := eventDataString(next.Data, "delta")
	if !strings.Contains(got, `{"path":"setting/outline.md"`) || !strings.Contains(got, "第一行") {
		t.Fatalf("non-chapter delta should restore held args, got %q", got)
	}
}

func TestSSEWriteChapterBodyMiddlewareKeepsConfigManagerWriteDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindConfigManager,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}})

	next := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindConfigManager,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"chapters/ch02.md","content":"第一行`,
	}})

	if got := eventDataString(next.Data, "delta"); !strings.Contains(got, "第一行") {
		t.Fatalf("config_manager delta should stay unchanged: %q", got)
	}
}

func TestSSEWriteChapterBodyMiddlewareKeepsAdditionalEditChapterDeltas(t *testing.T) {
	collector, handler := newWriteChapterBodySSETestHandler()
	_ = mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "edit",
		"args":       "",
	}})

	next := mustForwardSSEEvent(t, collector, handler, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "edit",
		"delta":      `{"path":"chapters/ch02.md","new_string":"第一行`,
	}})

	if got := eventDataString(next.Data, "delta"); !strings.Contains(got, "第一行") {
		t.Fatalf("edit delta should stay unchanged: %q", got)
	}
}

func TestIsNovelChapterBodyPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "relative chapter", path: "chapters/ch01.md", want: true},
		{name: "relative draft", path: "./drafts/ch01.md", want: true},
		{name: "absolute nova chapter", path: "/Users/me/nova/.nova/测试/chapters/ch01.md", want: true},
		{name: "absolute nova draft", path: `/Users\me\nova\.nova\测试\drafts\ch01.md`, want: true},
		{name: "absolute unrelated chapter directory", path: "/Users/me/tmp/chapters/ch01.md", want: false},
		{name: "nova setting", path: "/Users/me/nova/.nova/测试/setting/outline.md", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNovelChapterBodyPath(tc.path); got != tc.want {
				t.Fatalf("isNovelChapterBodyPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func newWriteChapterBodySSETestHandler() (*sseEventCollector, SSEEventHandler) {
	collector := &sseEventCollector{}
	chain := newSSEEventMiddlewareChainWithMiddlewares(newWriteChapterBodySSEMiddleware())
	return collector, chain.Next(collector.Handle)
}

func assertChapterBodyHiddenNotice(t *testing.T, data interface{}) {
	t.Helper()
	fields, ok := data.(map[string]interface{})["sse_hidden_fields"].([]string)
	if !ok || len(fields) != 1 || fields[0] != "content" {
		t.Fatalf("sse_hidden_fields = %#v, want [content]", data.(map[string]interface{})["sse_hidden_fields"])
	}
	if got := eventDataString(data, "sse_hidden_reason"); got != chapterBodyHiddenReason {
		t.Fatalf("sse_hidden_reason = %q, want %q", got, chapterBodyHiddenReason)
	}
	if got := eventDataString(data, "sse_display_notice"); got != chapterBodyHiddenNotice {
		t.Fatalf("sse_display_notice = %q, want %q", got, chapterBodyHiddenNotice)
	}
}

func assertGeneratedChars(t *testing.T, data interface{}, want int) {
	t.Helper()
	got, ok := data.(map[string]interface{})["sse_generated_chars"].(int)
	if !ok || got != want {
		t.Fatalf("sse_generated_chars = %#v, want %d", data.(map[string]interface{})["sse_generated_chars"], want)
	}
}

func fileCharCount(text string) int {
	return utf8.RuneCountInString(text)
}
