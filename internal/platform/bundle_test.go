package platform

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"denova/extensionassets"
)

func TestBundledPackageUsesNormalValidationAndExport(t *testing.T) {
	m, projectID := testManager(t)
	fixture := testCandidate(t, m, projectID, "static", "test.bundle", Game)
	source := fstest.MapFS{}
	for name, data := range fixture.files {
		source[name] = &fstest.MapFile{Data: data}
	}
	delete(source, "client.mjs")
	candidate, err := m.PreviewBundled(Game, source)
	if err != nil {
		t.Fatal(err)
	}
	client, err := extensionassets.Files().ReadFile("sdk/client.mjs")
	if err != nil || !bytes.Equal(candidate.files["client.mjs"], client) {
		t.Fatalf("shared browser client missing: %v", err)
	}
	installed, err := m.List(Game)
	if err != nil || len(installed) != 0 {
		t.Fatalf("preview installed a package: %#v %v", installed, err)
	}
	release := testInstall(t, m, candidate)
	var exported bytes.Buffer
	if err := m.ExportInstalled(release.Ref, &exported); err != nil {
		t.Fatal(err)
	}
	reimported, err := m.PreviewZIP(Game, exported.Bytes())
	if err != nil || reimported.Digest != candidate.Digest || !reflect.DeepEqual(reimported.Files, candidate.Files) {
		t.Fatalf("bundle export changed release bytes: %#v %v", reimported, err)
	}
	source["client.mjs"] = &fstest.MapFile{Data: []byte("// Extension-owned client")}
	custom, err := m.PreviewBundled(Game, source)
	if err != nil || !bytes.Equal(custom.files["client.mjs"], source["client.mjs"].Data) {
		t.Fatalf("extension client was overwritten: %v", err)
	}
	source["linked.mjs"] = &fstest.MapFile{Data: []byte("index.html"), Mode: os.ModeSymlink}
	if _, err := m.PreviewBundled(Game, source); err == nil {
		t.Fatal("bundle accepted a symlink")
	}
	delete(source, "linked.mjs")
	source["CLIENT.mjs"] = &fstest.MapFile{Data: []byte("case collision")}
	if _, err := m.PreviewBundled(Game, source); err == nil {
		t.Fatal("bundle accepted case-conflicting paths")
	}
}
