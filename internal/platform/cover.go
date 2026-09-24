package platform

import (
	"io"
	"net/http"
	"os"
	"strings"

	"denova/internal/portablepath"
)

// Covers are optional game presentation assets. They are distributed relative
// paths, never remote URLs, and are read without activating extension code.
const MaxCoverBytes = 4 << 20

func coverContentType(data []byte) (string, error) {
	if len(data) > MaxCoverBytes {
		return "", failure("LIMIT_EXCEEDED", "Game cover exceeds %d bytes", MaxCoverBytes)
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return contentType, nil
	default:
		return "", failure("INVALID_ARGUMENT", "Game cover must be a PNG, JPEG, GIF or WebP image")
	}
}

// GameCover returns only the declared image from an installed release. The
// caller must serve the returned media type with nosniff on the trusted origin.
func (m *Manager) GameCover(id, releaseID string) ([]byte, string, error) {
	if strings.HasPrefix(releaseID, "preview-") {
		return nil, "", failure("NOT_FOUND", "Game cover requires an installed release")
	}
	release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: id}, ReleaseID: releaseID})
	if err != nil {
		return nil, "", err
	}
	if release.Manifest.Game == nil || release.Manifest.Game.Cover == "" {
		return nil, "", failure("NOT_FOUND", "Game has no cover")
	}
	name := release.Manifest.Game.Cover
	if err := portablepath.Validate(name); err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(m.releasePath(release.Ref))
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxCoverBytes+1))
	if err != nil {
		return nil, "", err
	}
	contentType, err := coverContentType(data)
	return data, contentType, err
}
