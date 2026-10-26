package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// MaxChangelogBytes bounds a complete history, including large version gaps.
// An oversized source fails visibly rather than presenting a truncated range.
const MaxChangelogBytes = 4 << 20

// Changelog reads an already staged package when possible, including offline
// uploads. Otherwise it reads the target tag, never the mutable default branch.
func (s *Service) Changelog(ctx context.Context, version string) (string, error) {
	path, err := readPendingManifestRef(updateDataDir(filepath.Dir(s.executablePath)))
	if err == nil {
		manifest, readErr := readManifest(path)
		if readErr == nil && sameVersion(manifest.Version, version) {
			data, readErr := os.ReadFile(filepath.Join(manifest.SourceDir, "CHANGELOG.md"))
			if readErr == nil {
				if len(data) > MaxChangelogBytes {
					return "", fmt.Errorf("staged changelog exceeds %d bytes", MaxChangelogBytes)
				}
				return string(data), nil
			}
			if !errors.Is(readErr, os.ErrNotExist) {
				return "", fmt.Errorf("read staged changelog: %w", readErr)
			}
		}
	}
	s.ensureHTTPClient()
	// Static tag contents do not need GitHub API metadata or its shared quota.
	base := strings.TrimRight(s.githubRawBase, "/")
	endpoint := base + "/" + strings.Trim(s.repository, "/") + "/" + url.PathEscape("v"+normalizeVersion(version)) + "/CHANGELOG.md"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "denova-update-checker")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("read release changelog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("read release changelog: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxChangelogBytes+1))
	if err != nil {
		return "", fmt.Errorf("read release changelog: %w", err)
	}
	if len(data) > MaxChangelogBytes {
		return "", fmt.Errorf("release changelog exceeds %d bytes", MaxChangelogBytes)
	}
	return string(data), nil
}
