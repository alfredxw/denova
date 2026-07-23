package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"regexp"
	"strings"
)

var githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// GitHubSource is the user-provided source for a GitHub Skill repository.
type GitHubSource struct {
	URL    string `json:"url"`
	Ref    string `json:"ref,omitempty"`
	Subdir string `json:"subdir,omitempty"`
}

// GitHubRepository identifies a public GitHub repository archive to inspect.
type GitHubRepository struct {
	Owner  string
	Repo   string
	Ref    string
	Subdir string
}

func PreviewGitHub(ctx context.Context, dirs []Directory, scope Scope, source GitHubSource) (InstallPreview, error) {
	repo, err := ParseGitHubSource(source)
	if err != nil {
		return InstallPreview{}, err
	}
	data, err := DownloadGitHubArchive(ctx, repo)
	if err != nil {
		return InstallPreview{}, err
	}
	return previewZip(ctx, dirs, scope, data, repo.Subdir)
}

func InstallGitHub(ctx context.Context, dirs []Directory, scope Scope, source GitHubSource, candidateIDs []string) (InstallResult, error) {
	repo, err := ParseGitHubSource(source)
	if err != nil {
		return InstallResult{}, err
	}
	data, err := DownloadGitHubArchive(ctx, repo)
	if err != nil {
		return InstallResult{}, err
	}
	return installZip(ctx, dirs, scope, data, repo.Subdir, candidateIDs)
}

func ParseGitHubSource(source GitHubSource) (GitHubRepository, error) {
	raw := strings.TrimSpace(source.URL)
	if raw == "" {
		return GitHubRepository{}, fmt.Errorf("GitHub URL is required")
	}

	var repo GitHubRepository
	if shorthandOwner, shorthandRepo, ok := parseGitHubShorthand(raw); ok {
		repo.Owner = shorthandOwner
		repo.Repo = shorthandRepo
	} else {
		parsed, err := parseGitHubURL(raw)
		if err != nil {
			return GitHubRepository{}, err
		}
		repo = parsed
	}

	if ref := strings.TrimSpace(source.Ref); ref != "" {
		repo.Ref = strings.Trim(ref, "/")
	}
	if subdir := strings.Trim(strings.TrimSpace(source.Subdir), "/"); subdir != "" {
		repo.Subdir = subdir
	}
	if !validGitHubName(repo.Owner) || !validGitHubName(repo.Repo) {
		return GitHubRepository{}, fmt.Errorf("invalid GitHub repository: %s/%s", repo.Owner, repo.Repo)
	}
	return repo, nil
}

func DownloadGitHubArchive(ctx context.Context, repo GitHubRepository) ([]byte, error) {
	ref := strings.TrimSpace(repo.Ref)
	if ref == "" {
		defaultRef, err := resolveGitHubDefaultBranch(ctx, repo)
		if err != nil {
			return nil, err
		}
		ref = defaultRef
	}
	archiveURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball/%s",
		urlpkg.PathEscape(repo.Owner),
		urlpkg.PathEscape(repo.Repo),
		urlpkg.PathEscape(ref),
	)
	return downloadSkillArchive(ctx, archiveURL, "GitHub", map[string]string{
		"Accept": "application/vnd.github+json",
	})
}

func parseGitHubShorthand(raw string) (string, string, bool) {
	if strings.Contains(raw, "://") || strings.Contains(raw, "@") || strings.HasPrefix(raw, "github.com/") {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if !validGitHubName(owner) || !validGitHubName(repo) {
		return "", "", false
	}
	return owner, repo, true
}

func parseGitHubURL(raw string) (GitHubRepository, error) {
	if strings.HasPrefix(raw, "github.com/") {
		raw = "https://" + raw
	}
	parsed, err := urlpkg.Parse(raw)
	if err != nil {
		return GitHubRepository{}, err
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	if parsed.Scheme != "https" || host != "github.com" {
		return GitHubRepository{}, fmt.Errorf("only public https://github.com repositories are supported")
	}
	parts := splitURLPath(parsed.Path)
	if len(parts) < 2 {
		return GitHubRepository{}, fmt.Errorf("GitHub URL must include owner and repo")
	}
	repo := GitHubRepository{
		Owner: parts[0],
		Repo:  strings.TrimSuffix(parts[1], ".git"),
	}
	if len(parts) == 2 {
		return repo, nil
	}
	if len(parts) >= 4 && parts[2] == "tree" {
		repo.Ref = parts[3]
		if len(parts) > 4 {
			repo.Subdir = strings.Join(parts[4:], "/")
		}
		return repo, nil
	}
	return GitHubRepository{}, fmt.Errorf("unsupported GitHub URL path; use a repository URL or /tree/<ref>/<path>")
}

func splitURLPath(value string) []string {
	raw := strings.Split(strings.Trim(value, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func resolveGitHubDefaultBranch(ctx context.Context, repo GitHubRepository) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", urlpkg.PathEscape(repo.Owner), urlpkg.PathEscape(repo.Repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "denova-skill-installer")
	resp, err := skillInstallHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve GitHub default branch failed: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		DefaultBranch string `json:"default_branch"`
		Message       string `json:"message"`
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if readErr != nil {
		return "", readErr
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(body.Message)
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("resolve GitHub default branch failed: %s", msg)
	}
	ref := strings.TrimSpace(body.DefaultBranch)
	if ref == "" {
		return "", fmt.Errorf("GitHub repository default branch is empty")
	}
	return ref, nil
}

func validGitHubName(value string) bool {
	return githubNamePattern.MatchString(strings.TrimSpace(value))
}

func readSmallResponse(reader io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(reader, 4096))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}
