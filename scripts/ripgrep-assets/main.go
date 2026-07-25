// Command ripgrep-assets downloads and verifies the platform-specific
// ripgrep executable included in a Denova release package.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	ripgrepVersion        = "15.2.0"
	ripgrepReleaseBaseURL = "https://github.com/BurntSushi/ripgrep/releases/download/" + ripgrepVersion
	maxRipgrepArchiveSize = 32 * 1024 * 1024
)

type ripgrepAsset struct {
	Name           string
	SHA256         string
	ExecutableName string
}

var ripgrepAssets = map[string]ripgrepAsset{
	"darwin-arm64": {
		Name:           "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
		SHA256:         "3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4",
		ExecutableName: "rg",
	},
	"darwin-x64": {
		Name:           "ripgrep-15.2.0-x86_64-apple-darwin.tar.gz",
		SHA256:         "af7825fcc69a2afc7a7aea55fc9af90e26421d8f20fe59df32e233c0b8a231c1",
		ExecutableName: "rg",
	},
	"linux-arm64": {
		Name:           "ripgrep-15.2.0-aarch64-unknown-linux-musl.tar.gz",
		SHA256:         "800b1e7206afe799dfb5a6901f23147cfaabe0e52210538100f61e86e1740915",
		ExecutableName: "rg",
	},
	"linux-x64": {
		Name:           "ripgrep-15.2.0-x86_64-unknown-linux-musl.tar.gz",
		SHA256:         "33e15bcf1624b25cdd2a55813a47a2f95dbe126268203e76aa6a585d1e7b149c",
		ExecutableName: "rg",
	},
	"windows-x64": {
		Name:           "ripgrep-15.2.0-x86_64-pc-windows-msvc.zip",
		SHA256:         "71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5",
		ExecutableName: "rg.exe",
	},
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "package ripgrep: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, client *http.Client) error {
	flags := flag.NewFlagSet("ripgrep-assets", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("target", "", "Denova release target such as linux-x64")
	destination := flags.String("destination", "", "Denova package directory")
	baseURL := flags.String("base-url", ripgrepReleaseBaseURL, "ripgrep release asset base URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*destination) == "" {
		return errors.New("destination is required")
	}
	asset, err := resolveRipgrepAsset(*target)
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("HTTP client is required")
	}
	if err := packageRipgrep(ctx, client, *baseURL, asset, *destination); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ripgrep %s bundled for %s\n", ripgrepVersion, *target)
	return nil
}

func resolveRipgrepAsset(target string) (ripgrepAsset, error) {
	asset, ok := ripgrepAssets[strings.TrimSpace(target)]
	if !ok {
		return ripgrepAsset{}, fmt.Errorf("unsupported Denova release target %q", target)
	}
	return asset, nil
}

func packageRipgrep(ctx context.Context, client *http.Client, baseURL string, asset ripgrepAsset, destination string) error {
	archive, err := downloadRipgrepArchive(ctx, client, baseURL, asset.Name)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(archive)
	if actual := hex.EncodeToString(digest[:]); !strings.EqualFold(actual, asset.SHA256) {
		return fmt.Errorf("ripgrep archive checksum mismatch: got %s want %s", actual, asset.SHA256)
	}
	files, err := readRipgrepArchive(archive, asset)
	if err != nil {
		return err
	}
	return installRipgrepFiles(destination, asset.ExecutableName, files)
}

func downloadRipgrepArchive(ctx context.Context, client *http.Client, baseURL, assetName string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+assetName, nil)
	if err != nil {
		return nil, fmt.Errorf("create ripgrep download request: %w", err)
	}
	request.Header.Set("User-Agent", "denova-release-builder")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download ripgrep asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download ripgrep asset: HTTP %d", response.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxRipgrepArchiveSize+1))
	if err != nil {
		return nil, fmt.Errorf("read ripgrep asset: %w", err)
	}
	if len(archive) > maxRipgrepArchiveSize {
		return nil, fmt.Errorf("ripgrep asset exceeds %d bytes", maxRipgrepArchiveSize)
	}
	return archive, nil
}

type ripgrepArchiveFiles struct {
	Executable []byte
	MITLicense []byte
	Unlicense  []byte
}

func readRipgrepArchive(archive []byte, asset ripgrepAsset) (ripgrepArchiveFiles, error) {
	root, err := ripgrepArchiveRoot(asset.Name)
	if err != nil {
		return ripgrepArchiveFiles{}, err
	}
	wanted := map[string]*[]byte{
		path.Join(root, asset.ExecutableName): nil,
		path.Join(root, "LICENSE-MIT"):        nil,
		path.Join(root, "UNLICENSE"):          nil,
	}
	result := ripgrepArchiveFiles{}
	wanted[path.Join(root, asset.ExecutableName)] = &result.Executable
	wanted[path.Join(root, "LICENSE-MIT")] = &result.MITLicense
	wanted[path.Join(root, "UNLICENSE")] = &result.Unlicense
	if strings.HasSuffix(asset.Name, ".zip") {
		err = readRipgrepZip(archive, wanted)
	} else {
		err = readRipgrepTarGz(archive, wanted)
	}
	if err != nil {
		return ripgrepArchiveFiles{}, err
	}
	for name, content := range wanted {
		if content == nil || len(*content) == 0 {
			return ripgrepArchiveFiles{}, fmt.Errorf("ripgrep archive is missing %s", name)
		}
	}
	return result, nil
}

func ripgrepArchiveRoot(assetName string) (string, error) {
	switch {
	case strings.HasSuffix(assetName, ".tar.gz"):
		return strings.TrimSuffix(assetName, ".tar.gz"), nil
	case strings.HasSuffix(assetName, ".zip"):
		return strings.TrimSuffix(assetName, ".zip"), nil
	default:
		return "", fmt.Errorf("unsupported ripgrep archive %q", assetName)
	}
}

func readRipgrepZip(archive []byte, wanted map[string]*[]byte) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open ripgrep zip: %w", err)
	}
	for _, entry := range reader.File {
		target, ok := wanted[path.Clean(entry.Name)]
		if !ok {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open ripgrep zip entry %s: %w", entry.Name, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxRipgrepArchiveSize+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read ripgrep zip entry %s: %w", entry.Name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close ripgrep zip entry %s: %w", entry.Name, closeErr)
		}
		if len(content) > maxRipgrepArchiveSize {
			return fmt.Errorf("ripgrep zip entry %s is too large", entry.Name)
		}
		*target = content
	}
	return nil
}

func readRipgrepTarGz(archive []byte, wanted map[string]*[]byte) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open ripgrep tar.gz: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read ripgrep tar.gz: %w", err)
		}
		target, ok := wanted[path.Clean(header.Name)]
		if !ok || header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Size > maxRipgrepArchiveSize {
			return fmt.Errorf("ripgrep tar entry %s is too large", header.Name)
		}
		content, err := io.ReadAll(io.LimitReader(tarReader, maxRipgrepArchiveSize+1))
		if err != nil {
			return fmt.Errorf("read ripgrep tar entry %s: %w", header.Name, err)
		}
		if len(content) > maxRipgrepArchiveSize {
			return fmt.Errorf("ripgrep tar entry %s is too large", header.Name)
		}
		*target = content
	}
}

func installRipgrepFiles(destination, executableName string, files ripgrepArchiveFiles) error {
	toolsDir := filepath.Join(destination, "tools")
	licenseDir := filepath.Join(destination, "licenses", "ripgrep")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return fmt.Errorf("create runtime tools directory: %w", err)
	}
	if err := os.MkdirAll(licenseDir, 0o755); err != nil {
		return fmt.Errorf("create ripgrep license directory: %w", err)
	}
	outputs := []struct {
		path    string
		content []byte
		mode    os.FileMode
	}{
		{path: filepath.Join(toolsDir, executableName), content: files.Executable, mode: 0o755},
		{path: filepath.Join(licenseDir, "LICENSE-MIT"), content: files.MITLicense, mode: 0o644},
		{path: filepath.Join(licenseDir, "UNLICENSE"), content: files.Unlicense, mode: 0o644},
	}
	for _, output := range outputs {
		if err := os.WriteFile(output.path, output.content, output.mode); err != nil {
			return fmt.Errorf("write bundled ripgrep file %s: %w", output.path, err)
		}
		if err := os.Chmod(output.path, output.mode); err != nil {
			return fmt.Errorf("set bundled ripgrep mode %s: %w", output.path, err)
		}
	}
	return nil
}
