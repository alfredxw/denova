package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

type testMessageItem struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Summary     string  `json:"summary"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	ReadAt      *string `json:"read_at"`
}

func TestMessagesAPIListsAndMarksRead(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [Unreleased]

### Added

- 消息中心。

## [v0.1.17] - 2026-06-27

### Fixed

- 修复更新检查。
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOVA_CHANGELOG_PATH", changelog)

	application := newTestApplication(t)
	server := NewServer(application, "0")

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/messages", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Items       []testMessageItem `json:"items"`
		UnreadCount int               `json:"unread_count"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if listBody.UnreadCount != 1 || len(listBody.Items) != 1 {
		t.Fatalf("initial messages = %#v", listBody)
	}
	if !strings.HasPrefix(listBody.Items[0].ID, "changelog:v0.1.17:") || listBody.Items[0].ReadAt != nil {
		t.Fatalf("first message = %#v", listBody.Items[0])
	}

	summaryResp := performJSONRequest(t, server, http.MethodGet, "/api/activity/summary", nil)
	if summaryResp.Code != http.StatusOK {
		t.Fatalf("activity summary status = %d body=%s", summaryResp.Code, summaryResp.Body.String())
	}
	var summaryBody struct {
		MessageUnreadCount         int `json:"message_unread_count"`
		AutomationInboxUnreadCount int `json:"automation_inbox_unread_count"`
		AutomationRunningCount     int `json:"automation_running_count"`
	}
	decodeResponse(t, summaryResp.Body.Bytes(), &summaryBody)
	if summaryBody.MessageUnreadCount != 1 || summaryBody.AutomationInboxUnreadCount != 0 || summaryBody.AutomationRunningCount != 0 {
		t.Fatalf("initial activity summary = %#v", summaryBody)
	}
	if strings.Contains(summaryResp.Body.String(), `"items"`) || strings.Contains(summaryResp.Body.String(), `"body"`) {
		t.Fatalf("activity summary leaked full records: %s", summaryResp.Body.String())
	}

	readResp := performJSONRequest(t, server, http.MethodPost, "/api/messages/"+url.PathEscape(listBody.Items[0].ID)+"/read", nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResp.Code, readResp.Body.String())
	}
	var readItem testMessageItem
	decodeResponse(t, readResp.Body.Bytes(), &readItem)
	if readItem.ID != listBody.Items[0].ID || readItem.ReadAt == nil {
		t.Fatalf("read item = %#v", readItem)
	}

	nextResp := performJSONRequest(t, server, http.MethodGet, "/api/messages", nil)
	decodeResponse(t, nextResp.Body.Bytes(), &listBody)
	if listBody.UnreadCount != 0 || listBody.Items[0].ReadAt == nil {
		t.Fatalf("messages after read = %#v", listBody)
	}
	summaryResp = performJSONRequest(t, server, http.MethodGet, "/api/activity/summary", nil)
	decodeResponse(t, summaryResp.Body.Bytes(), &summaryBody)
	if summaryBody.MessageUnreadCount != 0 {
		t.Fatalf("activity summary after read = %#v", summaryBody)
	}

	readAllResp := performJSONRequest(t, server, http.MethodPost, "/api/messages/read-all", nil)
	if readAllResp.Code != http.StatusOK {
		t.Fatalf("read all status = %d body=%s", readAllResp.Code, readAllResp.Body.String())
	}
	decodeResponse(t, readAllResp.Body.Bytes(), &listBody)
	if listBody.UnreadCount != 0 || len(listBody.Items) != 1 {
		t.Fatalf("messages after read all = %#v", listBody)
	}
	for _, item := range listBody.Items {
		if item.ReadAt == nil {
			t.Fatalf("message should be read after read all: %#v", item)
		}
	}
}

func TestMessagesAPIUsesRequestLocale(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [v0.2.0] - 2026-07-01

### Brief / 简要说明

#### 中文

- 中文简要。

#### English

- English brief.

### Added

- 消息中心只展示中文更新。
- Message center only shows English updates.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOVA_CHANGELOG_PATH", changelog)

	application := newTestApplication(t)
	server := NewServer(application, "0")

	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/messages",
		nil,
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Denova-Locale", Value: "en-US"},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", resp.Code, resp.Body.String())
	}
	var listBody struct {
		Items       []testMessageItem `json:"items"`
		UnreadCount int               `json:"unread_count"`
	}
	decodeResponse(t, resp.Body.Bytes(), &listBody)
	if listBody.UnreadCount != 1 || len(listBody.Items) != 1 {
		t.Fatalf("messages = %#v", listBody)
	}
	item := listBody.Items[0]
	if item.Summary != "English brief." || !strings.Contains(item.Body, "Message center only shows English updates.") {
		t.Fatalf("English message missing expected content: %#v", item)
	}
	if strings.Contains(item.Body, "中文") || strings.Contains(item.Body, "消息中心") || strings.Contains(item.Body, "简要说明") {
		t.Fatalf("English message leaked Chinese content:\n%s", item.Body)
	}
}

func TestActivitySummarySupportsGzipWithoutCompressingEventStreams(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	compressed := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/activity/summary",
		nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip"},
	)
	if encoding := string(compressed.Header().Peek("Content-Encoding")); encoding != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", encoding)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	var summary map[string]int
	decodeResponse(t, body, &summary)
	if _, ok := summary["message_unread_count"]; !ok {
		t.Fatalf("decompressed activity summary = %#v", summary)
	}

	streamCompatible := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/activity/summary",
		nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip"},
		ut.Header{Key: "Accept", Value: "text/event-stream"},
	)
	if encoding := string(streamCompatible.Header().Peek("Content-Encoding")); encoding != "" {
		t.Fatalf("event-stream content encoding = %q, want empty", encoding)
	}

	// Browsers send Accept-Encoding automatically, while the AI SDK stream
	// transport does not need to send an explicit Accept header. Stream routes
	// must therefore be excluded by path as a server-side invariant.
	streamRoute := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/chat/stream?task_id=missing",
		nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip"},
	)
	if streamRoute.Code != http.StatusConflict {
		t.Fatalf("missing stream status = %d, want %d", streamRoute.Code, http.StatusConflict)
	}
	if encoding := string(streamRoute.Header().Peek("Content-Encoding")); encoding != "" {
		t.Fatalf("stream route content encoding = %q, want empty", encoding)
	}
}
