package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"denova/internal/styleref"
)

func TestStyleReferenceFileReadAndUpdateAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	ref, err := application.SaveStyleReference(styleref.WriteRequest{
		Name:     "克制文风",
		Filename: "restraint.md",
		Content:  "# 克制文风\n\n动作承载情绪。\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	readResp := performJSONRequest(t, server, http.MethodGet, "/api/styles/file?path="+url.QueryEscape(ref.DisplayPath), nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResp.Code, readResp.Body.String())
	}
	var readBody styleref.FileDocument
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	if readBody.Reference.DisplayPath != ref.DisplayPath || !strings.Contains(readBody.Content, "动作承载情绪") || readBody.Revision == "" {
		t.Fatalf("unexpected read body: %#v", readBody)
	}

	updateResp := performJSONRequest(t, server, http.MethodPut, "/api/styles/file", map[string]string{
		"path":          ref.DisplayPath,
		"content":       "# 锋利文风\n\n对白更锋利。",
		"base_revision": readBody.Revision,
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", updateResp.Code, updateResp.Body.String())
	}
	var updateBody styleref.FileDocument
	decodeResponse(t, updateResp.Body.Bytes(), &updateBody)
	if updateBody.Reference.Name != "锋利文风" || updateBody.Content != "# 锋利文风\n\n对白更锋利。\n" {
		t.Fatalf("unexpected update body: %#v", updateBody)
	}
}

func TestStyleReferenceFileUpdateRejectsStaleRevisionAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	ref, err := application.SaveStyleReference(styleref.WriteRequest{
		Name:     "旧文风",
		Filename: "stale.md",
		Content:  "# 旧文风\n\n旧内容。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := application.StyleReferenceFile(ref.DisplayPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref.Path, []byte("# 外部更新\n\n外部内容明显更长。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodPut, "/api/styles/file", map[string]string{
		"path":          ref.DisplayPath,
		"content":       "# 前端旧内容\n\n旧编辑器内容。",
		"base_revision": doc.Revision,
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("write status = %d body=%s", resp.Code, resp.Body.String())
	}
	got, err := os.ReadFile(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "外部更新") {
		t.Fatalf("stale update should keep external content, got:\n%s", string(got))
	}
}
