package api

import (
	"context"
	"net/http"
	"os"
	"testing"

	novaskills "denova/internal/skills"
)

func TestSkillDocumentUpdateRejectsStaleRevisionAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	doc, err := application.CreateSkillDocument(context.Background(), novaskills.ScopeUser, "revision-test", "original", nil)
	if err != nil {
		t.Fatal(err)
	}
	external := novaskills.DefaultContent("revision-test", "external update")
	if err := os.WriteFile(doc.Path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodPut, "/api/skills/document", map[string]string{
		"scope":         string(novaskills.ScopeUser),
		"name":          doc.Name,
		"content":       novaskills.DefaultContent(doc.Name, "stale editor"),
		"base_revision": doc.Revision,
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("write status = %d body=%s", resp.Code, resp.Body.String())
	}
	data, readErr := os.ReadFile(doc.Path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != external {
		t.Fatalf("stale API update overwrote external content: %q", data)
	}
}
