package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"denova/internal/project"
)

func TestSharedContentUsesProjectPackageIdentityAndRevisionedDocuments(t *testing.T) {
	m, projectID := testManager(t)
	m.ConfigureResources(&resourceTestHost{project: projectID, image: resourcePNG(t)})
	release := testResourceGame(t, m, projectID)
	_, first := openResourceGame(t, m, projectID, release)
	_, second := openResourceGame(t, m, projectID, release)
	input := ContentDocument{Path: "work-draft.json", Content: `{"title":"A work"}`}
	status, data := testRequest(t, first.Connection, "PUT", "/assets/document", "", input)
	if status != 200 {
		t.Fatalf("create document: %d %s", status, data)
	}
	var saved struct{ Revision string }
	_ = json.Unmarshal(data, &saved)
	if status, data := testRequest(t, second.Connection, "GET", "/assets/document?path=work-draft.json", "", nil); status != 200 || !bytes.Contains(data, []byte(saved.Revision)) {
		t.Fatalf("same Project work unavailable in another playthrough: %d %s", status, data)
	}
	if status, _ := testRequest(t, first.Connection, "PUT", "/assets/document", "", input); status != 409 {
		t.Fatalf("create replay overwrote content: %d", status)
	}
	input.ExpectedRevision = &saved.Revision
	input.Content = `{"title":"Revised"}`
	if status, data := testRequest(t, first.Connection, "PUT", "/assets/document", "", input); status != 200 {
		t.Fatalf("CAS update: %d %s", status, data)
	}
	if status, _ := testRequest(t, second.Connection, "PUT", "/assets/document", "", input); status != 409 {
		t.Fatalf("stale document update accepted: %d", status)
	}
	other, err := m.registry.Add(t.TempDir(), project.TypeGeneral, "Another Project")
	if err != nil {
		t.Fatal(err)
	}
	_, different := openResourceGame(t, m, other.ID, release)
	if status, _ := testRequest(t, different.Connection, "GET", "/assets/document?path=work-draft.json", "", nil); status != 404 {
		t.Fatalf("cross-Project authoring content leaked: %d", status)
	}
	for _, name := range []string{"../outside.json", "a/b.json", "work.js", "CON.json"} {
		if status, _ := testRequest(t, first.Connection, "GET", "/assets/document?path="+url.QueryEscape(name), "", nil); status != 400 {
			t.Fatalf("unsafe document %s accepted: %d", name, status)
		}
	}
}

func TestSharedMediaAdoptionIsIdempotentAndRejectsActiveContent(t *testing.T) {
	m, projectID := testManager(t)
	m.ConfigureResources(&resourceTestHost{project: projectID})
	release := testResourceGame(t, m, projectID)
	_, opened := openResourceGame(t, m, projectID, release)
	post := func(data []byte) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, opened.Connection.BaseURL+"/assets/upload", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+opened.Connection.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, body
	}
	image := resourcePNG(t)
	status, first := post(image)
	if status != 200 {
		t.Fatalf("adopt media: %d %s", status, first)
	}
	status, second := post(image)
	if status != 200 || !bytes.Equal(first, second) {
		t.Fatalf("media not content-addressed: %d %s", status, second)
	}
	var result GeneratedImage
	_ = json.Unmarshal(first, &result)
	if result.Asset.Kind != "shared" {
		t.Fatalf("incorrect material scope: %+v", result.Asset)
	}
	if status, data := testRequest(t, opened.Connection, "GET", "/assets/content?kind=shared&path="+result.Asset.Path, "", nil); status != 200 || !bytes.Equal(data, image) {
		t.Fatalf("read adopted material: %d %s", status, data)
	}
	if status, _ := post([]byte(`<svg><script>alert(1)</script></svg>`)); status != 409 {
		t.Fatalf("active content accepted: %d", status)
	}
}

func TestContentSourcePinsRepositoryAndVerifiesReleaseDigest(t *testing.T) {
	m, projectID := testManager(t)
	m.ConfigureResources(&resourceTestHost{project: projectID})
	release := testResourceGame(t, m, projectID)
	_, opened := openResourceGame(t, m, projectID, release)
	commit := strings.Repeat("a", 40)
	githubTestServer(t, m, &commit, map[string][]byte{commit: githubTestArchive(t, map[string][]byte{"novel/work.json": []byte(`{"name":"A"}`)})})
	status, data := testRequest(t, opened.Connection, "POST", "/assets/source", "", ContentSource{URL: "https://github.com/author/repo", Path: "novel"})
	files, err := readPackageZIP(data)
	if status != 200 || err != nil || string(files["work.json"]) != `{"name":"A"}` {
		t.Fatalf("repository content adapter: %d %v %s", status, err, data)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	m.githubHTTP.Transport = githubTestTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data)), Request: request}, nil
	})
	address := "https://github.com/author/repo/releases/download/v1/work.zip"
	if status, result := testRequest(t, opened.Connection, "POST", "/assets/source", "", ContentSource{URL: address, SHA256: digest}); status != 200 || !bytes.Equal(result, data) {
		t.Fatalf("release content: %d %s", status, result)
	}
	for _, source := range []ContentSource{{URL: address}, {URL: address, SHA256: strings.Repeat("0", 64)}, {URL: "https://localhost/private.zip", SHA256: digest}} {
		if status, _ := testRequest(t, opened.Connection, "POST", "/assets/source", "", source); status != 400 {
			t.Fatalf("invalid content source accepted: %d %+v", status, source)
		}
	}
}
