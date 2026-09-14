package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type resourceTestHost struct {
	project  string
	profile  string
	items    []LibraryItem
	image    []byte
	calls    atomic.Int32
	blocking bool
	entered  chan struct{}
}

func (h *resourceTestHost) LibraryItems(ctx context.Context, project string) ([]LibraryItem, error) {
	if project != h.project {
		return nil, failure("PERMISSION_DENIED", "Unexpected Project")
	}
	return h.items, ctx.Err()
}
func (h *resourceTestHost) ReadAsset(ctx context.Context, project, name string) ([]byte, error) {
	if project != h.project || name != "assets/portrait.png" {
		return nil, failure("NOT_FOUND", "Unknown asset")
	}
	return h.image, ctx.Err()
}
func (h *resourceTestHost) GenerateImage(ctx context.Context, project, profile string, _ ImageRequest) (ImageBytes, error) {
	h.calls.Add(1)
	if project != h.project || profile != h.profile {
		return ImageBytes{}, failure("PERMISSION_DENIED", "Unexpected Project or model binding")
	}
	if h.entered != nil {
		close(h.entered)
	}
	if h.blocking {
		<-ctx.Done()
		return ImageBytes{}, ctx.Err()
	}
	return ImageBytes{Data: h.image, RevisedPrompt: "A test image"}, nil
}

func testResourceGame(t *testing.T, m *Manager, project string) Release {
	t.Helper()
	development := testSource(t, m, project, "resource-game", "static", "test.resources", Game)
	_, layout, err := m.registry.Resolve(project, true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(layout.ContentRoot, "resource-game", "denova.game.json")
	var manifest Manifest
	if err := readJSON(path, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Permissions.Required = []string{"library.read", "assets.read", "images.generate"}
	manifest.ModelSlots = []ModelSlot{{ID: "art", TitleKey: "writer", Kind: "image", Required: true}}
	if err := writeJSON(path, manifest); err != nil {
		t.Fatal(err)
	}
	candidate, err := m.CheckDevelopment(development.ID)
	if err != nil {
		t.Fatal(err)
	}
	return testInstall(t, m, candidate)
}

func openResourceGame(t *testing.T, m *Manager, project string, release Release) (Instance, RuntimeSnapshot) {
	t.Helper()
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, ProjectID: project, Title: "Resource game", Models: map[string]string{"local:art": "test-image"}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	return instance, opened
}

func resourcePNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func awaitImage(t *testing.T, connection Connection, command string) ImageResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, data := testRequest(t, connection, "GET", "/images/generations/"+command, "", nil)
		if status != 200 {
			t.Fatalf("image status: %d %s", status, data)
		}
		var result ImageResult
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "running" {
			return result
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("image generation did not settle")
	return ImageResult{}
}

func TestResourceCapabilitiesScopePaginationAndRasterReads(t *testing.T) {
	m, project := testManager(t)
	host := &resourceTestHost{project: project, profile: "test-image", image: resourcePNG(t), items: []LibraryItem{
		{ID: "a", Name: "Alice", Tags: []string{"cast"}, Content: "Alice's story", Enabled: true, Image: &AssetRef{Kind: "project", Path: "assets/portrait.png"}},
		{ID: "b", Name: "Bob", Tags: []string{"cast"}, Content: "Bob's story", Enabled: true},
	}}
	m.ConfigureResources(host)
	release := testResourceGame(t, m, project)
	_, opened := openResourceGame(t, m, project, release)
	status, data := testRequest(t, opened.Connection, "GET", "/library/items?query=cast&limit=1", "", nil)
	var page struct {
		Items      []LibraryItem `json:"items"`
		Total      int           `json:"total"`
		NextOffset int           `json:"nextOffset"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if status != 200 || page.Total != 2 || page.NextOffset != 1 || len(page.Items) != 1 || page.Items[0].Content != "" || page.Items[0].ID != "a" {
		t.Fatalf("page: %d %s", status, data)
	}
	status, data = testRequest(t, opened.Connection, "GET", "/library/items/a", "", nil)
	var item LibraryItem
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(item, host.items[0]) {
		t.Fatalf("item: %d %s", status, data)
	}
	for _, route := range []string{"/library/items?offset=-1", "/library/items?limit=101", "/library/items?limit=bad", "/assets/content?kind=project&path=../outside.png"} {
		if status, data := testRequest(t, opened.Connection, "GET", route, "", nil); status != 400 {
			t.Fatalf("invalid route %s: %d %s", route, status, data)
		}
	}
	status, data = testRequest(t, opened.Connection, "GET", "/assets/content?kind=project&path=assets/portrait.png", "", nil)
	if status != 200 || !bytes.Equal(data, host.image) {
		t.Fatalf("asset: %d %s", status, data)
	}
	if status, _ := testRequest(t, opened.Connection, "GET", "/assets/content?kind=project&path=secrets.png", "", nil); status != 403 {
		t.Fatalf("non-asset read: %d", status)
	}
	if status, _ := testRequest(t, opened.Connection, "GET", "/assets/content?kind=generated&path=missing.png", "", nil); status != 404 {
		t.Fatalf("missing generated image: %d", status)
	}
	ungranted := testInstall(t, m, testCandidate(t, m, project, "static", "test.no-resources", Game))
	_, denied := openResourceGame(t, m, project, ungranted)
	if status, _ := testRequest(t, denied.Connection, "GET", "/library/items", "", nil); status != 403 {
		t.Fatalf("missing grant: %d", status)
	}
	invalidToken := opened.Connection
	invalidToken.Token = "untrusted"
	if status, _ := testRequest(t, invalidToken, "GET", "/library/items", "", nil); status != 403 {
		t.Fatalf("invalid credential: %d", status)
	}
	host.items[0].Content = strings.Repeat("x", MaxLibraryItemBytes+1)
	if status, _ := testRequest(t, opened.Connection, "GET", "/library/items/a", "", nil); status != 413 {
		t.Fatalf("oversized item: %d", status)
	}
}

func TestImageCommandsReplayAndAssetsRemainScopeIsolated(t *testing.T) {
	m, project := testManager(t)
	host := &resourceTestHost{project: project, profile: "test-image", image: resourcePNG(t)}
	m.ConfigureResources(host)
	release := testResourceGame(t, m, project)
	instance, opened := openResourceGame(t, m, project, release)
	command := ImageRequest{CommandID: "portrait-1", ModelSlot: "art", Prompt: "A portrait"}
	if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 202 {
		t.Fatalf("start: %d %s", status, data)
	}
	result := awaitImage(t, opened.Connection, command.CommandID)
	if result.Status != "completed" || len(result.Images) != 1 {
		t.Fatalf("result: %#v", result)
	}
	asset := result.Images[0].Asset
	route := "/assets/content?kind=" + asset.Kind + "&path=" + url.QueryEscape(asset.Path)
	if status, data := testRequest(t, opened.Connection, "GET", route, "", nil); status != 200 || !bytes.Equal(data, host.image) {
		t.Fatalf("generated asset: %d", status)
	}
	_, other := openResourceGame(t, m, project, release)
	if status, _ := testRequest(t, other.Connection, "GET", route, "", nil); status != 404 {
		t.Fatalf("cross-scope image: %d", status)
	}
	if status, _ := testRequest(t, other.Connection, "GET", "/images/generations/portrait-1", "", nil); status != 404 {
		t.Fatalf("cross-scope receipt: %d", status)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.ConfigureResources(host)
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command)
	var replay ImageResult
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(replay, result) || host.calls.Load() != 1 {
		t.Fatalf("replay: %d %s calls=%d", status, data, host.calls.Load())
	}
	command.Prompt = "Different portrait"
	if status, _ := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 409 {
		t.Fatalf("idempotency conflict: %d", status)
	}
	command.CommandID, command.ModelSlot = "other", "undeclared"
	if status, _ := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 403 {
		t.Fatalf("undeclared slot: %d", status)
	}
}

func TestImageCancelAndUncertainRecoveryNeverResubmit(t *testing.T) {
	m, project := testManager(t)
	host := &resourceTestHost{project: project, profile: "test-image", image: resourcePNG(t), blocking: true, entered: make(chan struct{})}
	m.ConfigureResources(host)
	release := testResourceGame(t, m, project)
	instance, opened := openResourceGame(t, m, project, release)
	command := ImageRequest{CommandID: "cancel-me", ModelSlot: "art", Prompt: "A portrait"}
	if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 202 {
		t.Fatalf("start: %d %s", status, data)
	}
	select {
	case <-host.entered:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	if status, _ := testRequest(t, opened.Connection, "POST", "/images/generations/cancel-me/cancel", "", nil); status != 200 {
		t.Fatalf("cancel: %d", status)
	}
	if result := awaitImage(t, opened.Connection, command.CommandID); result.Status != "cancelled" {
		t.Fatalf("cancelled result: %#v", result)
	}
	if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 200 || host.calls.Load() != 1 {
		t.Fatalf("cancel replay: %d %s", status, data)
	}
	command.CommandID = "uncertain"
	encoded, _ := json.Marshal([]any{command, "test-image"})
	receipt := imageReceipt{Version: 1, InputHash: stableID(string(encoded)), Result: ImageResult{CommandID: command.CommandID, Status: "running", Images: []GeneratedImage{}}}
	key := imageReceiptPath(m.runtimes[instance.ID].owner, command.CommandID)
	if err := writeJSON(key, receipt); err != nil {
		t.Fatal(err)
	}
	status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command)
	var result ImageResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if status != 200 || result.Status != "interrupted" || host.calls.Load() != 1 {
		t.Fatalf("uncertain replay: %d %s", status, data)
	}
	host.entered = make(chan struct{})
	command.CommandID = "runtime-stop"
	if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 202 {
		t.Fatalf("start before runtime stop: %d %s", status, data)
	}
	select {
	case <-host.entered:
	case <-time.After(time.Second):
		t.Fatal("provider did not start before runtime stop")
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitImage(t, opened.Connection, command.CommandID); result.Status != "cancelled" || host.calls.Load() != 2 {
		t.Fatalf("runtime stop did not settle image before reopen: %#v calls=%d", result, host.calls.Load())
	}
}

func TestRasterAssetRejectsUnsafePathsAndActiveContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "portrait.png"), resourcePNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "active.svg"), []byte("<svg><script>alert(1)</script></svg>"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../portrait.png", "Portrait.png", "active.svg"} {
		if _, err := ReadRasterAsset(root, name); err == nil {
			t.Fatalf("unsafe asset accepted: %s", name)
		}
	}
	if _, err := ReadRasterAsset(root, "portrait.png"); err != nil {
		t.Fatal(err)
	}
}
