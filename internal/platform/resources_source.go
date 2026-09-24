package platform

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// ContentSource downloads public GitHub content without installing or executing
// it. The consuming extension validates its own content format. Repository
// refs are resolved to an exact commit using the existing source adapter.
type ContentSource struct {
	URL    string `json:"url"`
	Ref    string `json:"ref,omitempty"`
	Path   string `json:"path,omitempty"`
	Commit string `json:"commit,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

func (s *ResourceService) serveContentSource(w http.ResponseWriter, request *http.Request, runtime *Runtime) {
	if request.Method != http.MethodPost {
		writeError(w, failure("NOT_FOUND", "Unknown content source method"))
		return
	}
	var input ContentSource
	if err := readRequest(request, &input); err != nil {
		writeError(w, err)
		return
	}
	address, err := url.Parse(input.URL)
	if err != nil || address.Scheme != "https" || address.Host != "github.com" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		writeError(w, failure("INVALID_ARGUMENT", "Content sources require a public GitHub HTTPS URL"))
		return
	}
	parts := strings.Split(strings.Trim(address.Path, "/"), "/")
	var data []byte
	if len(parts) >= 6 && parts[2] == "releases" && parts[3] == "download" && githubName.MatchString(parts[0]) && githubName.MatchString(parts[1]) && strings.HasSuffix(parts[len(parts)-1], ".zip") {
		if input.SHA256 == "" {
			writeError(w, failure("INVALID_ARGUMENT", "Release downloads require the expected ZIP SHA-256"))
			return
		}
		client := *runtime.manager.githubHTTP
		client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 || next.URL.Scheme != "https" || next.URL.User != nil || next.URL.Port() != "" || !slices.Contains([]string{"github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"}, next.URL.Hostname()) {
				return failure("PERMISSION_DENIED", "Release redirected outside GitHub asset origins")
			}
			return nil
		}
		var outgoing *http.Request
		outgoing, err = http.NewRequestWithContext(request.Context(), http.MethodGet, input.URL, nil)
		if err == nil {
			outgoing.Header.Set("User-Agent", "Denova")
			var response *http.Response
			response, err = client.Do(outgoing)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					err = failure("RUNTIME_UNAVAILABLE", "GitHub release returned HTTP %d", response.StatusCode)
				} else if response.ContentLength > MaxPackageBytes {
					err = failure("LIMIT_EXCEEDED", "Release exceeds package size limit")
				} else {
					data, err = io.ReadAll(io.LimitReader(response.Body, MaxPackageBytes+1))
				}
			}
		}
	} else {
		var source GitHubSource
		var files map[string][]byte
		source, files, err = runtime.manager.downloadGitHubSource(request.Context(), GitHubSource{URL: input.URL, Ref: input.Ref, Path: input.Path, Commit: input.Commit})
		if err == nil {
			selected := map[string][]byte{}
			names := []string{}
			for name, content := range files {
				if source.Path == "." || strings.HasPrefix(name, source.Path+"/") {
					name = strings.TrimPrefix(name, source.Path+"/")
					selected[name], names = content, append(names, name)
				}
			}
			slices.Sort(names)
			var buffer bytes.Buffer
			err = writePackageArchive(Candidate{Files: names, files: selected}, &buffer)
			data = buffer.Bytes()
			w.Header().Set("X-Denova-Source-Commit", source.Commit)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if len(data) > MaxPackageBytes {
		writeError(w, failure("LIMIT_EXCEEDED", "Source exceeds package size limit"))
		return
	}
	sum := sha256.Sum256(data)
	digest := fmt.Sprintf("%x", sum[:])
	if input.SHA256 != "" && !strings.EqualFold(input.SHA256, digest) {
		writeError(w, failure("INVALID_ARGUMENT", "Source ZIP SHA-256 does not match"))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("X-Denova-SHA256", digest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
