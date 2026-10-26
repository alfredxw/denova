package messages

import "strings"

// ReleaseNotes is a localized, published changelog entry for reading. Brief and
// Body do not overlap; older entries without a Brief show their complete body.
type ReleaseNotes struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Brief       string `json:"brief"`
	Body        string `json:"body,omitempty"`
}

// ParseReleaseNotes shares the notification parser without its feed limits.
// Callers bound the source size and select the requested version range.
func ParseReleaseNotes(content, locale string) []ReleaseNotes {
	items := parseChangelog(content, locale, 0)
	releases := make([]ReleaseNotes, 0, len(items))
	for _, item := range items {
		brief, body := releaseBrief(item.Body)
		releases = append(releases, ReleaseNotes{
			Version: item.Title, PublishedAt: item.PublishedAt,
			Summary: item.Summary, Brief: brief, Body: body,
		})
	}
	return releases
}

func releaseBrief(body string) (string, string) {
	lines := strings.Split(body, "\n")
	for start, line := range lines {
		if line != "### Brief" && line != "### 简要说明" {
			continue
		}
		end := start + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "### ") {
			end++
		}
		remaining := append(append([]string{}, lines[:start]...), lines[end:]...)
		return trimBlankLines(strings.Join(lines[start+1:end], "\n")), trimBlankLines(strings.Join(remaining, "\n"))
	}
	return body, ""
}
