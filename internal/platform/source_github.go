package platform

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// GitHubSource describes a public source checkout. Ref tracks upstream; Commit
// fixes the downloaded input, while the package digest identifies built bytes.
// Path is relative to the repository root, never a host filesystem location.
type GitHubSource struct {
	URL    string `json:"url"`
	Ref    string `json:"ref"`
	Path   string `json:"path"`
	Commit string `json:"commit,omitempty"`
}

type GitHubUpdate struct {
	Status string       `json:"status"`
	Source GitHubSource `json:"source"`
}

var githubName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var githubCommit = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func newGitHubClient() *http.Client {
	// Fixed HTTPS origins let source downloads use the host's proxy settings
	// without exposing a general-purpose URL fetcher through management APIs.
	return &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !githubDownloadURL(request.URL) {
			return failure("GITHUB_UNAVAILABLE", "GitHub redirected outside the supported download origins")
		}
		return nil
	}}
}

func githubDownloadURL(value *url.URL) bool {
	if value.Scheme != "https" || value.User != nil || value.Port() != "" {
		return false
	}
	switch value.Hostname() {
	case "api.github.com", "github.com", "codeload.github.com":
		return true
	default:
		return false
	}
}

func normalizeGitHubSource(source GitHubSource) (GitHubSource, error) {
	value, err := url.Parse(strings.TrimSpace(source.URL))
	if err != nil || value.Scheme != "https" || !strings.EqualFold(value.Host, "github.com") || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return source, failure("GITHUB_URL_INVALID", "Expected a public https://github.com/owner/repository URL")
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value.Path, "/"), "/"), "/")
	if len(parts) != 2 {
		return source, failure("GITHUB_URL_INVALID", "Specify the repository URL and use the separate ref and path fields")
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	if !githubName.MatchString(parts[0]) || !githubName.MatchString(parts[1]) || len(parts[0]) > 39 || len(parts[1]) > 100 {
		return source, failure("GITHUB_URL_INVALID", "Invalid GitHub repository identity")
	}
	source.URL = "https://github.com/" + strings.ToLower(strings.Join(parts, "/"))
	source.Ref = strings.TrimSpace(source.Ref)
	if len(source.Ref) > 256 || strings.ContainsFunc(source.Ref, unicode.IsControl) {
		return source, failure("GITHUB_REF_INVALID", "Invalid GitHub branch, tag or commit")
	}
	source.Path = strings.TrimSpace(source.Path)
	if source.Path == "" {
		source.Path = "."
	}
	if err := validateDevelopmentPath(source.Path); err != nil {
		return source, failure("GITHUB_PATH_INVALID", "Invalid repository package path: %v", err)
	}
	if source.Commit != "" && !githubCommit.MatchString(source.Commit) {
		return source, failure("GITHUB_REF_INVALID", "Expected a full Git commit ID")
	}
	source.Commit = strings.ToLower(source.Commit)
	return source, nil
}

func (m *Manager) githubGet(ctx context.Context, address, mediaType string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	if !githubDownloadURL(request.URL) {
		return nil, failure("GITHUB_URL_INVALID", "Unsupported GitHub download origin")
	}
	request.Header.Set("Accept", mediaType)
	request.Header.Set("User-Agent", "Denova")
	response, err := m.githubHTTP.Do(request)
	if err != nil {
		return nil, failure("GITHUB_UNAVAILABLE", "GitHub request failed: %v", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, failure("GITHUB_NOT_FOUND", "GitHub repository or ref is unavailable: %s", address)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, failure("GITHUB_RATE_LIMIT", "GitHub rejected the request with HTTP %d", response.StatusCode)
	default:
		return nil, failure("GITHUB_UNAVAILABLE", "GitHub returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, failure("LIMIT_EXCEEDED", "GitHub response exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, failure("GITHUB_UNAVAILABLE", "GitHub download failed: %v", err)
	}
	if int64(len(data)) > limit {
		return nil, failure("LIMIT_EXCEEDED", "GitHub response exceeds %d bytes", limit)
	}
	return data, nil
}

func (m *Manager) resolveGitHubSource(ctx context.Context, source GitHubSource) (GitHubSource, error) {
	source, err := normalizeGitHubSource(source)
	if err != nil {
		return source, err
	}
	endpoint := "https://api.github.com/repos/" + strings.TrimPrefix(source.URL, "https://github.com/")
	if source.Ref == "" {
		data, err := m.githubGet(ctx, endpoint, "application/vnd.github+json", MaxDefinitionBytes)
		if err != nil {
			return source, err
		}
		var repository struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.Unmarshal(data, &repository); err != nil || repository.DefaultBranch == "" {
			return source, failure("GITHUB_UNAVAILABLE", "GitHub did not return a default branch")
		}
		source.Ref = repository.DefaultBranch
	}
	ref := source.Commit
	if ref == "" {
		ref = source.Ref
	}
	// Request only the SHA, avoiding potentially large commit diffs.
	data, err := m.githubGet(ctx, endpoint+"/commits/"+url.PathEscape(ref), "application/vnd.github.sha", 1024)
	if err != nil {
		return source, err
	}
	commit := strings.TrimSpace(string(data))
	if !githubCommit.MatchString(commit) {
		return source, failure("GITHUB_UNAVAILABLE", "GitHub did not return a valid commit")
	}
	source.Commit = strings.ToLower(commit)
	return source, nil
}

func (m *Manager) downloadGitHubSource(ctx context.Context, source GitHubSource) (GitHubSource, map[string][]byte, error) {
	source, data, err := m.downloadGitHubArchive(ctx, source)
	if err != nil {
		return source, nil, err
	}
	files, err := readGitHubArchive(data)
	return source, files, err
}

func (m *Manager) downloadGitHubArchive(ctx context.Context, source GitHubSource) (GitHubSource, []byte, error) {
	source, err := m.resolveGitHubSource(ctx, source)
	if err != nil {
		return source, nil, err
	}
	address := "https://api.github.com/repos/" + strings.TrimPrefix(source.URL, "https://github.com/") + "/zipball/" + source.Commit
	data, err := m.githubGet(ctx, address, "application/vnd.github+json", MaxPackageBytes)
	if err != nil {
		return source, nil, err
	}
	slog.Info("platform_github_source_downloaded", "repository", source.URL, "ref", source.Ref, "commit", source.Commit, "path", source.Path)
	return source, data, nil
}

// PreviewGitHub only downloads and validates files. Build commands run solely
// through the existing visible workbench terminal after an explicit import.
func (m *Manager) PreviewGitHub(ctx context.Context, source GitHubSource) (Candidate, error) {
	source, files, err := m.downloadGitHubSource(ctx, source)
	if err != nil {
		return Candidate{}, err
	}
	kind, manifest, files, err := githubPackageRoot(source.Path, files)
	if err != nil {
		return Candidate{}, err
	}
	files, err = githubDistribution(kind, manifest, files)
	if err != nil {
		return Candidate{}, err
	}
	candidate, err := m.freeze(kind, files)
	if err != nil {
		return Candidate{}, err
	}
	m.mu.Lock()
	m.candidates[candidate.ID].Source = &source
	candidate.Source = &source
	m.mu.Unlock()
	return candidate, nil
}

func (m *Manager) CheckGitHubUpdate(ctx context.Context, ref PackageRef) (GitHubUpdate, error) {
	_, item, err := m.currentPackage(ref.Kind, ref.ID)
	if err != nil {
		return GitHubUpdate{}, err
	}
	if item.Source == nil {
		return GitHubUpdate{}, failure("UNSUPPORTED", "Extension has no GitHub update source")
	}
	source := *item.Source
	source.Commit = ""
	source, err = m.resolveGitHubSource(ctx, source)
	if err != nil {
		return GitHubUpdate{}, err
	}
	status := "current"
	if source.Commit != item.Source.Commit {
		status = "available"
	}
	slog.Info("platform_github_update_checked", "kind", ref.Kind, "package", ref.ID, "status", status, "commit", source.Commit)
	return GitHubUpdate{Status: status, Source: source}, nil
}

// PreviewGitHubUpdate fixes the reviewed commit and binds installation to the
// current package. A remote rename or an intervening install cannot replace it.
func (m *Manager) PreviewGitHubUpdate(ctx context.Context, ref PackageRef, commit string) (Candidate, error) {
	current, item, err := m.currentPackage(ref.Kind, ref.ID)
	if err != nil {
		return Candidate{}, err
	}
	if item.Source == nil || !githubCommit.MatchString(commit) {
		return Candidate{}, failure("GITHUB_REF_INVALID", "A GitHub update requires an installed source and reviewed commit")
	}
	source := *item.Source
	source.Commit = commit
	candidate, err := m.PreviewGitHub(ctx, source)
	if err != nil {
		return Candidate{}, err
	}
	if candidate.Kind != ref.Kind || candidate.Manifest.ID != ref.ID {
		m.DiscardCandidate(candidate.ID)
		return Candidate{}, failure("GITHUB_IDENTITY_CHANGED", "GitHub update changed the extension identity")
	}
	m.mu.Lock()
	m.candidates[candidate.ID].replaces = &current.Ref
	m.mu.Unlock()
	return candidate, nil
}
