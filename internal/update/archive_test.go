package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadedArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
		size uint64
	}{
		{"../outside", 0o644, 0},
		{"/absolute", 0o644, 0},
		{`denova\..\outside`, 0o644, 0},
		{"denova/file:stream", 0o644, 0},
		{"denova/link", os.ModeSymlink | 0o777, 0},
		{"denova/huge", 0o644, 2<<30 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			header := &zip.FileHeader{Name: tc.name, UncompressedSize64: tc.size}
			header.SetMode(tc.mode)
			if _, err := writer.CreateRaw(header); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			path := filepath.Join(root, "upload.zip")
			if err := os.WriteFile(path, archive.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := extractArchive(path, filepath.Join(root, "extract")); err == nil {
				t.Fatal("unsafe entry was accepted")
			}
		})
	}
}

func TestUploadedTarRejectsSymlinksAndExpandedSize(t *testing.T) {
	for _, header := range []*tar.Header{
		{Name: "denova/link", Typeflag: tar.TypeSymlink, Linkname: "../outside"},
		{Name: "denova/huge", Typeflag: tar.TypeReg, Size: 2<<30 + 1},
	} {
		var archive bytes.Buffer
		gz := gzip.NewWriter(&archive)
		writer := tar.NewWriter(gz)
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		// No expanded payload is needed: the header must be rejected first.
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		path := filepath.Join(root, "upload.tar.gz")
		if err := os.WriteFile(path, archive.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := extractArchive(path, filepath.Join(root, "extract")); err == nil {
			t.Fatal("unsafe tar entry was accepted")
		}
	}
}
