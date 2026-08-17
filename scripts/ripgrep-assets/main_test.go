package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRipgrepAssetsCoverDenovaReleaseTargets(t *testing.T) {
	type expectedAsset struct {
		name       string
		checksum   string
		executable string
	}
	want := map[string]expectedAsset{
		"darwin-arm64": {
			name:       "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
			checksum:   "3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4",
			executable: "rg",
		},
		"darwin-x64": {
			name:       "ripgrep-15.2.0-x86_64-apple-darwin.tar.gz",
			checksum:   "af7825fcc69a2afc7a7aea55fc9af90e26421d8f20fe59df32e233c0b8a231c1",
			executable: "rg",
		},
		"linux-arm64": {
			name:       "ripgrep-15.2.0-aarch64-unknown-linux-musl.tar.gz",
			checksum:   "800b1e7206afe799dfb5a6901f23147cfaabe0e52210538100f61e86e1740915",
			executable: "rg",
		},
		"linux-x64": {
			name:       "ripgrep-15.2.0-x86_64-unknown-linux-musl.tar.gz",
			checksum:   "33e15bcf1624b25cdd2a55813a47a2f95dbe126268203e76aa6a585d1e7b149c",
			executable: "rg",
		},
		"windows-x64": {
			name:       "ripgrep-15.2.0-x86_64-pc-windows-msvc.zip",
			checksum:   "71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5",
			executable: "rg.exe",
		},
	}
	for target, expected := range want {
		asset, err := resolveRipgrepAsset(target)
		if err != nil {
			t.Fatalf("resolve %s: %v", target, err)
		}
		if asset.Name != expected.name || asset.SHA256 != expected.checksum || asset.ExecutableName != expected.executable {
			t.Fatalf("asset %s = %#v, want %#v", target, asset, expected)
		}
	}
	if _, err := resolveRipgrepAsset("freebsd-x64"); err == nil {
		t.Fatal("unsupported target should fail")
	}
}

func TestPackageRipgrepVerifiesAndInstallsTarAndZipAssets(t *testing.T) {
	for _, archiveType := range []string{"tar.gz", "zip"} {
		t.Run(archiveType, func(t *testing.T) {
			executableName := "rg"
			if archiveType == "zip" {
				executableName = "rg.exe"
			}
			assetName := "ripgrep-test-platform." + archiveType
			rootName := strings.TrimSuffix(strings.TrimSuffix(assetName, ".gz"), ".tar")
			rootName = strings.TrimSuffix(rootName, ".zip")
			archive := ripgrepFixtureArchive(t, archiveType, rootName, executableName)
			digest := sha256.Sum256(archive)
			asset := ripgrepAsset{
				Name:           assetName,
				SHA256:         hex.EncodeToString(digest[:]),
				ExecutableName: executableName,
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(archive)
			}))
			defer server.Close()

			destination := t.TempDir()
			if err := packageRipgrep(context.Background(), server.Client(), server.URL, asset, destination); err != nil {
				t.Fatal(err)
			}
			assertFixtureFile(t, filepath.Join(destination, "tools", executableName), "ripgrep binary")
			assertFixtureFile(t, filepath.Join(destination, "licenses", "ripgrep", "LICENSE-MIT"), "MIT license")
			assertFixtureFile(t, filepath.Join(destination, "licenses", "ripgrep", "UNLICENSE"), "Unlicense")
			if archiveType == "tar.gz" {
				info, err := os.Stat(filepath.Join(destination, "tools", executableName))
				if err != nil || info.Mode().Perm()&0o111 == 0 {
					t.Fatalf("bundled ripgrep is not executable: info=%v err=%v", info, err)
				}
			}
		})
	}
}

func TestPackageRipgrepRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not the pinned archive")
	}))
	defer server.Close()
	err := packageRipgrep(context.Background(), server.Client(), server.URL, ripgrepAsset{
		Name:           "ripgrep-test.tar.gz",
		SHA256:         strings.Repeat("0", sha256.Size*2),
		ExecutableName: "rg",
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("checksum mismatch error = %v", err)
	}
}

func ripgrepFixtureArchive(t *testing.T, archiveType, rootName, executableName string) []byte {
	t.Helper()
	files := map[string]string{
		filepath.ToSlash(filepath.Join(rootName, executableName)): "ripgrep binary",
		filepath.ToSlash(filepath.Join(rootName, "LICENSE-MIT")):  "MIT license",
		filepath.ToSlash(filepath.Join(rootName, "UNLICENSE")):    "Unlicense",
	}
	var buffer bytes.Buffer
	if archiveType == "zip" {
		writer := zip.NewWriter(&buffer)
		for name, content := range files {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(entry, content); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}

	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		mode := int64(0o644)
		if filepath.Base(name) == executableName {
			mode = 0o755
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFixtureFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, content, want)
	}
}
