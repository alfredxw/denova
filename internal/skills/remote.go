package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
)

var skillInstallHTTPClient = http.DefaultClient

// RemoteArchiveSource is the user-provided source for a remote Skill archive.
// GitHub repository references are resolved through the GitHub archive API;
// other sources must be absolute HTTPS archive URLs.
type RemoteArchiveSource struct {
	URL    string `json:"url"`
	Ref    string `json:"ref,omitempty"`
	Subdir string `json:"subdir,omitempty"`
}

func PreviewRemoteArchive(ctx context.Context, dirs []Directory, scope Scope, source RemoteArchiveSource) (InstallPreview, error) {
	data, subdir, err := remoteArchiveData(ctx, source)
	if err != nil {
		return InstallPreview{}, err
	}
	return previewZip(ctx, dirs, scope, data, subdir)
}

func InstallRemoteArchive(ctx context.Context, dirs []Directory, scope Scope, source RemoteArchiveSource, candidateIDs []string) (InstallResult, error) {
	data, subdir, err := remoteArchiveData(ctx, source)
	if err != nil {
		return InstallResult{}, err
	}
	return installZip(ctx, dirs, scope, data, subdir, candidateIDs)
}

func remoteArchiveData(ctx context.Context, source RemoteArchiveSource) ([]byte, string, error) {
	raw := strings.TrimSpace(source.URL)
	if raw == "" {
		return nil, "", fmt.Errorf("remote Skill archive URL is required")
	}

	if repo, ok, err := githubRepositoryFromRemoteSource(source); ok || err != nil {
		if err != nil {
			return nil, "", err
		}
		data, err := DownloadGitHubArchive(ctx, repo)
		return data, repo.Subdir, err
	}

	if strings.TrimSpace(source.Ref) != "" {
		return nil, "", fmt.Errorf("ref is only supported for GitHub repository sources")
	}
	data, err := DownloadRemoteArchive(ctx, raw)
	if err != nil {
		return nil, "", err
	}
	return data, normalizeArchiveSubdir(source.Subdir), nil
}

func githubRepositoryFromRemoteSource(source RemoteArchiveSource) (GitHubRepository, bool, error) {
	raw := strings.TrimSpace(source.URL)
	if raw == "" {
		return GitHubRepository{}, false, nil
	}
	if _, _, ok := parseGitHubShorthand(raw); ok {
		repo, err := ParseGitHubSource(GitHubSource(source))
		return repo, true, err
	}
	if strings.HasPrefix(raw, "github.com/") {
		repo, err := ParseGitHubSource(GitHubSource(source))
		return repo, true, err
	}
	parsed, err := urlpkg.Parse(raw)
	if err != nil {
		return GitHubRepository{}, false, nil
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	if parsed.Scheme != "https" || host != "github.com" {
		return GitHubRepository{}, false, nil
	}
	parts := splitURLPath(parsed.Path)
	if len(parts) == 2 || (len(parts) >= 4 && parts[2] == "tree") {
		repo, err := ParseGitHubSource(GitHubSource(source))
		return repo, true, err
	}
	if repo, ok := parseGitHubArchiveURLPath(parts); ok {
		applyRemoteSourceOverrides(&repo, source)
		return repo, true, nil
	}
	return GitHubRepository{}, false, nil
}

func parseGitHubArchiveURLPath(parts []string) (GitHubRepository, bool) {
	if len(parts) < 4 || parts[2] != "archive" {
		return GitHubRepository{}, false
	}
	ref := strings.Join(parts[3:], "/")
	if !strings.HasSuffix(ref, ".zip") {
		return GitHubRepository{}, false
	}
	ref = strings.TrimSuffix(ref, ".zip")
	if ref == "" {
		return GitHubRepository{}, false
	}
	repo := GitHubRepository{
		Owner: parts[0],
		Repo:  strings.TrimSuffix(parts[1], ".git"),
		Ref:   ref,
	}
	if !validGitHubName(repo.Owner) || !validGitHubName(repo.Repo) {
		return GitHubRepository{}, false
	}
	return repo, true
}

func applyRemoteSourceOverrides(repo *GitHubRepository, source RemoteArchiveSource) {
	if ref := strings.TrimSpace(source.Ref); ref != "" {
		repo.Ref = strings.Trim(ref, "/")
	}
	if subdir := normalizeArchiveSubdir(source.Subdir); subdir != "" {
		repo.Subdir = subdir
	}
}

func DownloadRemoteArchive(ctx context.Context, archiveURL string) ([]byte, error) {
	parsed, err := urlpkg.Parse(strings.TrimSpace(archiveURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("remote Skill archive URL must be an absolute https:// URL")
	}
	return downloadSkillArchive(ctx, parsed.String(), "Remote", map[string]string{
		"Accept": "application/zip, application/octet-stream, */*",
	})
}

func downloadSkillArchive(ctx context.Context, archiveURL, sourceLabel string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	req.Header.Set("User-Agent", "denova-skill-installer")
	resp, err := skillInstallHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s Skill archive failed: %w", sourceLabel, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s Skill archive failed: HTTP %d: %s", sourceLabel, resp.StatusCode, readSmallResponse(resp.Body))
	}
	if resp.ContentLength > MaxInstallArchiveBytes {
		return nil, archiveTooLargeError(sourceLabel)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxInstallArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxInstallArchiveBytes {
		return nil, archiveTooLargeError(sourceLabel)
	}
	if err := validateDownloadedZipArchive(data, sourceLabel); err != nil {
		return nil, err
	}
	return data, nil
}

func archiveTooLargeError(sourceLabel string) error {
	return fmt.Errorf("%s Skill archive must be %dMB or smaller", sourceLabel, MaxInstallArchiveBytes/(1024*1024))
}

func normalizeArchiveSubdir(subdir string) string {
	return strings.Trim(strings.TrimSpace(subdir), "/")
}

func validateDownloadedZipArchive(data []byte, sourceLabel string) error {
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
		return fmt.Errorf("download %s Skill archive failed: response is not a ZIP archive; use a GitHub repository/tree URL such as owner/repo, or an HTTPS URL that downloads a .zip file directly", sourceLabel)
	}
	return nil
}
