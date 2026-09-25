package platform

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/internal/project"
)

type githubTestTransport func(*http.Request) (*http.Response, error)

func (transport githubTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func githubTestArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range files {
		file, err := writer.Create("owner-repo-commit/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func githubTestServer(t *testing.T, manager *Manager, head *string, archives map[string][]byte) *[]string {
	t.Helper()
	requests := []string{}
	manager.githubHTTP.Transport = githubTestTransport(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		response := &http.Response{StatusCode: 200, Header: http.Header{}, Request: request}
		var data []byte
		switch {
		case request.URL.Path == "/repos/author/repo":
			data = []byte(`{"default_branch":"main"}`)
		case strings.HasPrefix(request.URL.Path, "/repos/author/repo/commits/"):
			ref := strings.TrimPrefix(request.URL.Path, "/repos/author/repo/commits/")
			sha := *head
			if githubCommit.MatchString(ref) {
				sha = ref
			}
			if request.Header.Get("Accept") != "application/vnd.github.sha" {
				t.Fatal("update check unnecessarily downloads commit diffs")
			}
			data = []byte(sha)
		case strings.HasPrefix(request.URL.Path, "/repos/author/repo/zipball/"):
			response.StatusCode = 302
			response.Header.Set("Location", "https://codeload.github.com/author/repo/"+strings.TrimPrefix(request.URL.Path, "/repos/author/repo/zipball/"))
		case request.URL.Host == "codeload.github.com":
			data = archives[strings.TrimPrefix(request.URL.Path, "/author/repo/")]
			if data == nil {
				response.StatusCode = 404
			}
		default:
			t.Fatalf("unexpected GitHub request: %s", request.URL)
		}
		response.Body = io.NopCloser(bytes.NewReader(data))
		return response, nil
	})
	return &requests
}

func TestGitHubInstallAndUpdateUseReviewedBytes(t *testing.T) {
	for _, kind := range []Kind{Plugin, Game} {
		t.Run(string(kind), func(t *testing.T) {
			m, projectID := testManager(t)
			fixture := "tool"
			if kind == Game {
				fixture = "static"
			}
			original := testCandidate(t, m, projectID, fixture, "test.github", kind)
			original.Manifest.Distribution.Files = original.Files
			original.files[kind.manifestFile()], _ = json.Marshal(original.Manifest)
			original, err := m.freeze(kind, original.files)
			if err != nil {
				t.Fatal(err)
			}
			firstSHA, secondSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
			head := firstSHA
			files := maps.Clone(original.files)
			files["README.md"] = []byte("Source-only file")
			archives := map[string][]byte{firstSHA: githubTestArchive(t, files)}
			requests := githubTestServer(t, m, &head, archives)
			candidate, err := m.PreviewGitHub(context.Background(), GitHubSource{URL: "https://github.com/Author/Repo.git"})
			if err != nil {
				t.Fatal(err)
			}
			wantSource := &GitHubSource{URL: "https://github.com/author/repo", Ref: "main", Path: ".", Commit: firstSHA}
			if candidate.Digest != original.Digest || !reflect.DeepEqual(candidate.Source, wantSource) {
				t.Fatalf("GitHub changed distribution or source metadata: %+v", candidate)
			}
			installed := testInstall(t, m, candidate)
			ref := installed.Ref.Package
			update, err := m.CheckGitHubUpdate(context.Background(), ref)
			if err != nil || update.Status != "current" {
				t.Fatalf("incorrect initial update state: %+v %v", update, err)
			}
			files["index.html"] = append(bytes.Clone(files["index.html"]), []byte("\n<!-- updated -->")...)
			// Both fixtures distribute index.html, retaining their declared version.
			archives[secondSHA] = githubTestArchive(t, files)
			head = secondSHA
			update, err = m.CheckGitHubUpdate(context.Background(), ref)
			if err != nil || update.Status != "available" || update.Source.Commit != secondSHA {
				t.Fatalf("missing upstream update: %+v %v", update, err)
			}
			head = strings.Repeat("c", 40)
			next, err := m.PreviewGitHubUpdate(context.Background(), ref, update.Source.Commit)
			if err != nil || next.Source.Commit != secondSHA || next.Digest == candidate.Digest {
				t.Fatalf("update did not freeze reviewed commit: %+v %v", next, err)
			}
			requestCount := len(*requests)
			updated := testInstall(t, m, next)
			if len(*requests) != requestCount || updated.Manifest.Version != installed.Manifest.Version {
				t.Fatal("installation fetched unreviewed bytes or changed the author version")
			}
			_, item, err := m.currentPackage(kind, ref.ID)
			if err != nil || item.Source.Commit != secondSHA || len(item.Releases) != 2 {
				t.Fatalf("incorrect installed update: %+v %v", item, err)
			}
			if _, _, err := m.release(installed.Ref); err != nil {
				t.Fatal("update removed the previous snapshot", err)
			}
			if _, err := m.Install(next.ID, next.Manifest.Permissions.Required); err == nil {
				t.Fatal("stale update replaced an intervening installation")
			}
		})
	}
}

func TestGitHubRejectsChangedIdentityAndPreservesInstallation(t *testing.T) {
	m, projectID := testManager(t)
	original := testCandidate(t, m, projectID, "static", "test.identity", Game)
	head := strings.Repeat("a", 40)
	archives := map[string][]byte{head: githubTestArchive(t, original.files)}
	githubTestServer(t, m, &head, archives)
	candidate, err := m.PreviewGitHub(context.Background(), GitHubSource{URL: "https://github.com/author/repo"})
	if err != nil {
		t.Fatal(err)
	}
	installed := testInstall(t, m, candidate)
	files := maps.Clone(original.files)
	manifest := original.Manifest
	manifest.ID = "test.replaced"
	files[Game.manifestFile()], _ = json.Marshal(manifest)
	head = strings.Repeat("b", 40)
	archives[head] = githubTestArchive(t, files)
	_, err = m.PreviewGitHubUpdate(context.Background(), installed.Ref.Package, head)
	_, problem := ErrorResponse(err)
	if problem.Code != "GITHUB_IDENTITY_CHANGED" {
		t.Fatalf("unexpected identity error: %v", err)
	}
	current, _, err := m.currentPackage(Game, installed.Manifest.ID)
	if err != nil || current.Ref != installed.Ref {
		t.Fatal("failed update changed current installation", err)
	}
}

func TestGitHubImportsBuildSourceAsPortableProject(t *testing.T) {
	m, projectID := testManager(t)
	original := testCandidate(t, m, projectID, "static", "test.build", Game)
	manifest := original.Manifest
	manifest.Distribution.Files = append(manifest.Distribution.Files, "dist")
	files := map[string][]byte{"package.json": []byte(`{"scripts":{"build":"echo explicit build"}}`)}
	for name, data := range original.files {
		files["games/demo/"+name] = data
	}
	files["games/demo/"+Game.manifestFile()], _ = json.Marshal(manifest)
	head := strings.Repeat("d", 40)
	githubTestServer(t, m, &head, map[string][]byte{head: githubTestArchive(t, files)})
	source := GitHubSource{URL: "https://github.com/author/repo", Ref: "feature/test", Path: "games/demo"}
	_, err := m.PreviewGitHub(context.Background(), source)
	_, problem := ErrorResponse(err)
	if problem.Code != "GITHUB_BUILD_REQUIRED" {
		t.Fatalf("missing build was not explained: %v", err)
	}
	imported, err := m.ImportGitHub(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	_, layout, err := m.registry.Resolve(imported.ProjectID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.ContentRoot, "package.json")); err != nil {
		t.Fatal("import lost repository-level build inputs", err)
	}
	metadata, err := os.ReadFile(filepath.Join(layout.StoreRoot, "development.json"))
	if err != nil || bytes.Contains(metadata, []byte(m.root)) {
		t.Fatalf("source provenance contains a host path: %s %v", metadata, err)
	}
	if catalog, err := m.Catalog(); err != nil || len(catalog) != 0 {
		t.Fatal("source import installed or ran a package", err)
	}
	// Author/build output is checked through the same workbench path after restart.
	if err := writeBytes(filepath.Join(layout.ContentRoot, "games", "demo", "dist", "built.js"), []byte("export {}")); err != nil {
		t.Fatal(err)
	}
	reloaded := New(m.root, project.NewRegistry(m.root))
	built, err := reloaded.CheckDevelopment(imported.ID)
	if err != nil || built.Source != nil {
		t.Fatalf("local build claimed upstream release identity: %+v %v", built, err)
	}
	testInstall(t, reloaded, built)
}

func TestGitHubInputAndDownloadBoundaries(t *testing.T) {
	for _, input := range []GitHubSource{
		{URL: "http://github.com/owner/repo"}, {URL: "https://github.com.evil.test/owner/repo"},
		{URL: "https://token@github.com/owner/repo"}, {URL: "https://github.com/owner/repo/tree/main"},
		{URL: "https://github.com/owner/repo", Path: "../outside"}, {URL: "https://github.com/owner/repo", Path: "CON"},
		{URL: "https://github.com/owner/repo", Commit: "main"},
	} {
		if _, err := normalizeGitHubSource(input); err == nil {
			t.Fatalf("accepted invalid source: %+v", input)
		}
	}
	for _, target := range []string{"http://codeload.github.com/file", "https://127.0.0.1/file", "https://github.com.evil.test/file"} {
		m, _ := testManager(t)
		m.githubHTTP.Transport = githubTestTransport(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
		})
		if _, err := m.resolveGitHubSource(context.Background(), GitHubSource{URL: "https://github.com/author/repo"}); err == nil {
			t.Fatalf("followed unsupported redirect: %s", target)
		}
	}
	for _, files := range []map[string][]byte{
		{"../outside": []byte("bad")}, {"A/x": nil, "a/y": nil}, {"file": nil, "file/child": nil},
	} {
		if _, err := readGitHubArchive(githubTestArchive(t, files)); err == nil {
			t.Fatalf("accepted unsafe source archive: %v", files)
		}
	}
}

func TestResourceSourceAdmitsOnlySelectedPortableSubtree(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		path      string
		mode      os.FileMode
		wantError bool
	}{
		{"unrelated link", "CLAUDE.md", os.ModeSymlink | 0777, false},
		{"selected link", "skills/example/reference.md", os.ModeSymlink | 0777, true},
		{"selected traversal", "skills/example/../../outside", 0600, true},
		{"selected case conflict", "skills/example/skill.md", 0600, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			regular, err := writer.Create("owner-repo-commit/skills/example/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := regular.Write([]byte("Portable Skill")); err != nil {
				t.Fatal(err)
			}
			header := &zip.FileHeader{Name: "owner-repo-commit/" + scenario.path}
			header.SetMode(scenario.mode)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte("AGENTS.md")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			m, _ := testManager(t)
			head := strings.Repeat("a", 40)
			githubTestServer(t, m, &head, map[string][]byte{head: buffer.Bytes()})
			resolved, files, err := m.SourceFiles(context.Background(), GitHubSource{URL: "https://github.com/author/repo", Ref: "main", Path: "skills/example"})
			if scenario.wantError {
				if err == nil {
					t.Fatal("accepted unsafe file inside selected subtree")
				}
				return
			}
			if err != nil || resolved.Commit != head || len(files) != 1 || string(files["skills/example/SKILL.md"]) != "Portable Skill" {
				t.Fatalf("subtree: %+v %v", files, err)
			}
			if _, err := readGitHubArchive(buffer.Bytes()); err == nil {
				t.Fatal("full checkout unexpectedly admitted a symlink")
			}
		})
	}
}
