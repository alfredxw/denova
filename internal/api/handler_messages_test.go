package api

import (
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
