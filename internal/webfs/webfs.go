// Package webfs optionally embeds the built frontend so the Nova binary can
// serve it without a web/ directory on disk.
//
// Build with the "embedweb" tag (after the frontend has been built and copied
// to ./dist, which build.sh does) to embed the assets:
//
//	pnpm --dir web build && cp -r web/dist internal/webfs/dist && go build -tags embedweb ./cmd/nova
//
// Without the tag (the default, e.g. for development), no assets are embedded
// and the app serves the frontend from an on-disk web directory as before.
package webfs

import (
	"io/fs"
	"os"
	"path/filepath"
)

// HasEmbedded reports whether the binary was built with embedded web assets.
func HasEmbedded() bool { return hasEmbedded }

// ExtractEmbedded writes the embedded assets to a fresh temp directory and
// returns its path, so the existing file-based static handler can serve them.
// Returns "" when no assets are embedded.
func ExtractEmbedded() (string, error) {
	if !hasEmbedded {
		return "", nil
	}
	root, err := os.MkdirTemp("", "nova-web-*")
	if err != nil {
		return "", err
	}
	// //go:embed all:dist keeps the "dist/" prefix in the tree; strip it so the
	// extracted files land at root/index.html (what the static handler expects).
	tree, subErr := fs.Sub(embeddedFS, "dist")
	if subErr != nil {
		os.RemoveAll(root)
		return "", subErr
	}
	walkErr := fs.WalkDir(tree, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(root, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := fs.ReadFile(tree, path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	})
	if walkErr != nil {
		os.RemoveAll(root)
		return "", walkErr
	}
	return root, nil
}
