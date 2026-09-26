package lore

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRemoteMaterialsPreserveSharingCoverAndLocalCopy(t *testing.T) {
	ctx := context.Background()
	s := NewStore(t.TempDir())
	for _, id := range []string{"a", "b"} {
		if _, err := s.Create(ItemInput{ID: id, Name: id, Content: "Keep text"}); err != nil {
			t.Fatal(err)
		}
	}
	const remote = "https://images.example.com/portrait?signature=keep%2Fthis"
	a, err := s.RemoteMaterial(ctx, "a", MaterialMutation{Op: "remote", URL: remote, Name: "Portrait", Description: "Keep face"})
	if err != nil {
		t.Fatal(err)
	}
	asset := a.ResolvedMaterials[0]
	if asset.URL != remote || asset.Path != "" || asset.MIMEType != "image/*" {
		t.Fatalf("invalid remote material: %+v", asset)
	}
	if _, err := os.Stat(filepath.Join(s.workspace, "assets")); !os.IsNotExist(err) {
		t.Fatal("online reference created files", err)
	}
	if _, err := s.MutateMaterial("a", MaterialMutation{Op: "cover", AssetID: asset.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MutateMaterial("b", MaterialMutation{Op: "link", AssetID: asset.ID, Name: "Cameo"}); err != nil {
		t.Fatal(err)
	}
	a, err = s.ReadAny("a")
	if err != nil || a.Image.ImageURL != remote || a.Image.ImagePath != "" {
		t.Fatalf("remote cover lost: %+v %v", a, err)
	}
	raw, _ := os.ReadFile(s.itemsPath())
	paths, err := MaterialFilePaths(raw)
	if err != nil || len(paths) != 0 {
		t.Fatal("remote references became file dependencies", paths, err)
	}
	// A download commits only the selected association, with the current text.
	if _, err := s.MutateMaterial("a", MaterialMutation{Op: "update", AssetID: asset.ID, Name: "Edited while downloading", Description: "Latest description"}); err != nil {
		t.Fatal(err)
	}
	a, err = s.SaveMaterial(ctx, "a", MaterialFile{Filename: "portrait.png", Data: materialPNG(t), Source: asset.Source, ReplaceAssetID: asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	local := a.ResolvedMaterials[0]
	if local.ID == asset.ID || local.Path == "" || local.URL != "" || local.Source.URL != remote || local.Name != "Edited while downloading" || local.Description != "Latest description" || a.Materials.CoverAssetID != local.ID || a.Content != "Keep text" {
		t.Fatalf("bad local replacement: %+v", a)
	}
	b, err := s.ReadAny("b")
	if err != nil || b.ResolvedMaterials[0].ID != asset.ID || b.ResolvedMaterials[0].URL != remote || b.ResolvedMaterials[0].Name != "Cameo" {
		t.Fatalf("shared reference changed: %+v %v", b, err)
	}
	before, _ := os.ReadFile(s.itemsPath())
	if _, err := s.SaveMaterial(ctx, "a", MaterialFile{Filename: "late.png", Data: materialPNG(t), Source: asset.Source, ReplaceAssetID: asset.ID}); err == nil {
		t.Fatal("stale replacement accepted")
	}
	after, _ := os.ReadFile(s.itemsPath())
	dirs, _ := os.ReadDir(filepath.Join(s.workspace, "assets/lore/media"))
	if !bytes.Equal(before, after) || len(dirs) != 1 {
		t.Fatal("failed replacement changed data or leaked files")
	}
}

func TestRemoteMaterialURLReplacementOnlyChangesSelectedAssociation(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, id := range []string{"a", "b"} {
		if _, err := s.Create(ItemInput{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.RemoteMaterial(context.Background(), "a", MaterialMutation{Op: "remote", URL: "https://images.example.com/old.png", Name: "Keep name"})
	if err != nil {
		t.Fatal(err)
	}
	id := a.ResolvedMaterials[0].ID
	if _, err = s.MutateMaterial("a", MaterialMutation{Op: "cover", AssetID: id}); err != nil {
		t.Fatal(err)
	}
	b, err := s.MutateMaterial("b", MaterialMutation{Op: "link", AssetID: id})
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.RemoteMaterial(context.Background(), "a", MaterialMutation{Op: "remote", AssetID: id, URL: "https://images.example.com/new.png"})
	if err != nil || a.ResolvedMaterials[0].Name != "Keep name" || a.Image.ImageURL != "https://images.example.com/new.png" {
		t.Fatal(a, err)
	}
	other, err := s.ReadAny("b")
	if err != nil || !reflect.DeepEqual(b, other) {
		t.Fatal("replacement changed another item", err)
	}
}

func TestRemoteMaterialDownloadValidatesBytesAndPublicDestination(t *testing.T) {
	data := materialPNG(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/image":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
		case "/large":
			w.Header().Set("Content-Length", "67108865")
		case "/audio":
			_, _ = w.Write(materialWAV())
		default:
			_, _ = w.Write([]byte("<html>Not an image</html>"))
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		path string
		want error
	}{{"/image", nil}, {"/large", ErrMaterialTooLarge}, {"/audio", ErrMaterialRemoteImage}, {"/page", ErrMaterialRemoteImage}} {
		got, err := downloadMaterial(context.Background(), server.Client(), server.URL+tc.path)
		if !errors.Is(err, tc.want) || tc.want == nil && !bytes.Equal(got, data) {
			t.Fatalf("%s: %v", tc.path, err)
		}
	}
	if _, err := downloadMaterial(context.Background(), materialHTTPClient(), server.URL+"/image"); !errors.Is(err, ErrMaterialDownload) {
		t.Fatal("private destination allowed", err)
	}
	for _, raw := range []string{"http://example.com/a.png", "https://user:pass@example.com/a.png", "file:///a.png", "javascript:alert(1)", "https:///a.png"} {
		if _, err := parseMaterialURL(raw); !errors.Is(err, ErrMaterialURL) {
			t.Fatalf("accepted %q", raw)
		}
		request, _ := http.NewRequest(http.MethodGet, raw, nil)
		if request != nil && materialHTTPClient().CheckRedirect(request, nil) == nil {
			t.Fatalf("accepted redirect %q", raw)
		}
	}
}
