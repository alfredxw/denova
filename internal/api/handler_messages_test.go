package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if listBody.UnreadCount != 2 || len(listBody.Items) != 2 {
		t.Fatalf("initial messages = %#v", listBody)
	}
	if !strings.HasPrefix(listBody.Items[0].ID, "changelog:unreleased:") || listBody.Items[0].ReadAt != nil {
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
	if listBody.UnreadCount != 1 || listBody.Items[0].ReadAt == nil {
		t.Fatalf("messages after read = %#v", listBody)
	}

	readAllResp := performJSONRequest(t, server, http.MethodPost, "/api/messages/read-all", nil)
	if readAllResp.Code != http.StatusOK {
		t.Fatalf("read all status = %d body=%s", readAllResp.Code, readAllResp.Body.String())
	}
	decodeResponse(t, readAllResp.Body.Bytes(), &listBody)
	if listBody.UnreadCount != 0 || len(listBody.Items) != 2 {
		t.Fatalf("messages after read all = %#v", listBody)
	}
	for _, item := range listBody.Items {
		if item.ReadAt == nil {
			t.Fatalf("message should be read after read all: %#v", item)
		}
	}
}
