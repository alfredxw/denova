package platform

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"testing"
)

func TestGameCoverValidationAndInstalledRead(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.cover", Game)
	var cover bytes.Buffer
	if err := png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		t.Fatal(err)
	}
	candidate.Manifest.Game.Cover = "images/cover.png"
	candidate.files["images/cover.png"] = cover.Bytes()
	candidate.files[Game.manifestFile()], _ = json.Marshal(candidate.Manifest)
	validated, err := m.freeze(Game, candidate.files)
	if err != nil {
		t.Fatal(err)
	}
	release := testInstall(t, m, validated)
	data, contentType, err := m.GameCover(release.Manifest.ID, release.Ref.ReleaseID)
	if err != nil || contentType != "image/png" || !bytes.Equal(data, cover.Bytes()) {
		t.Fatalf("cover: %s %v", contentType, err)
	}
	if len(m.runtimes) != 0 {
		t.Fatal("reading a cover started a runtime")
	}
	for _, path := range []string{"../cover.png", "/cover.png", "C:/cover.png", "https://example.com/cover.png", "missing.png"} {
		candidate.Manifest.Game.Cover = path
		candidate.files[Game.manifestFile()], _ = json.Marshal(candidate.Manifest)
		if _, err := validateManifest(Game, candidate.files); err == nil {
			t.Fatalf("accepted cover reference %s", path)
		}
	}
	candidate.Manifest.Game.Cover = "images/cover.png"
	candidate.files[Game.manifestFile()], _ = json.Marshal(candidate.Manifest)
	for _, invalid := range [][]byte{[]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), bytes.Repeat([]byte("x"), MaxCoverBytes+1)} {
		candidate.files["images/cover.png"] = invalid
		if _, err := validateManifest(Game, candidate.files); err == nil {
			t.Fatal("accepted invalid cover content")
		}
	}
	// An edited installed record cannot turn the read endpoint into a file reader.
	items, err := m.List(Game)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Releases[0].Manifest.Game.Cover = "../secret.txt"
	if err := m.saveInstalled(Game, items[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.GameCover(release.Manifest.ID, release.Ref.ReleaseID); err == nil {
		t.Fatal("accepted tampered cover path")
	}
}
