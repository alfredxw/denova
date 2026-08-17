package skills

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const skillReferenceScheme = "skill"
const defaultSkillReferenceLines = 2000

// ReadReference resolves a skill://<name>/references/<path> URI against the
// effective catalog for this Backend and returns a bounded line selection.
// It deliberately exposes neither storage scope nor editable document state.
func (b *Backend) ReadReference(ctx context.Context, rawURI string, offset, limit int) (ReferenceRead, error) {
	if b == nil {
		return ReferenceRead{}, fmt.Errorf("skill reference backend is nil")
	}
	if err := ctx.Err(); err != nil {
		return ReferenceRead{}, err
	}
	name, referencePath, canonicalURI, err := parseReferenceURI(rawURI)
	if err != nil {
		return ReferenceRead{}, err
	}
	rec, ok := b.activeRecord(ctx, name)
	if !ok {
		return ReferenceRead{}, fmt.Errorf("skill is not available to this Agent: %s", name)
	}
	rel, _, err := safeSkillFilePath(rec.skill.BaseDirectory, referencePath)
	if err != nil {
		return ReferenceRead{}, err
	}
	if rel == "references" || !strings.HasPrefix(rel, "references/") {
		return ReferenceRead{}, fmt.Errorf("skill reference must identify a file under references/: %s", rawURI)
	}

	root, err := openScopedSkillRoot(rec.directory, name)
	if err != nil {
		return ReferenceRead{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return ReferenceRead{}, fmt.Errorf("open skill reference %s: %w", canonicalURI, err)
	}
	defer file.Close()
	info, err := regularSkillFileInfoFromFile(file, rel)
	if err != nil {
		return ReferenceRead{}, err
	}
	if info.Size() > maxSkillFileBytes {
		return ReferenceRead{}, fmt.Errorf("skill reference is too large to read: %s", canonicalURI)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSkillFileBytes+1))
	if err != nil {
		return ReferenceRead{}, fmt.Errorf("read skill reference %s: %w", canonicalURI, err)
	}
	if int64(len(data)) > maxSkillFileBytes {
		return ReferenceRead{}, fmt.Errorf("skill reference is too large to read: %s", canonicalURI)
	}
	if !utf8.Valid(data) {
		return ReferenceRead{}, fmt.Errorf("skill reference is not valid UTF-8 text: %s", canonicalURI)
	}
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultSkillReferenceLines
	}
	content, total := selectReferenceLines(string(data), offset, limit)
	return ReferenceRead{
		URI: canonicalURI, Content: content, Offset: offset, Limit: limit, Total: total,
	}, nil
}

func (b *Backend) activeRecord(ctx context.Context, name string) (record, bool) {
	for _, rec := range b.activeRecords(ctx) {
		if rec.skill.Name == name {
			return rec, true
		}
	}
	return record{}, false
}

func parseReferenceURI(rawURI string) (name, referencePath, canonical string, err error) {
	rawURI = strings.TrimSpace(rawURI)
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", "", "", fmt.Errorf("parse skill reference URI: %w", err)
	}
	if parsed.Scheme != skillReferenceScheme || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", "", fmt.Errorf("skill reference must use skill://<name>/references/<path>")
	}
	name = parsed.Host
	if err := ValidateName(name); err != nil {
		return "", "", "", fmt.Errorf("invalid skill reference name: %w", err)
	}
	if strings.Contains(parsed.Path, "\\") {
		return "", "", "", fmt.Errorf("invalid skill reference path")
	}
	referencePath = strings.TrimPrefix(parsed.Path, "/")
	cleaned := path.Clean(referencePath)
	if cleaned == "." || cleaned == "references" || !strings.HasPrefix(cleaned, "references/") {
		return "", "", "", fmt.Errorf("skill reference must identify a file under references/: %s", rawURI)
	}
	if cleaned != referencePath {
		return "", "", "", fmt.Errorf("skill reference path must be canonical: %s", rawURI)
	}
	canonical = (&url.URL{Scheme: skillReferenceScheme, Host: name, Path: "/" + cleaned}).String()
	return name, cleaned, canonical, nil
}

func selectReferenceLines(content string, offset, limit int) (string, int) {
	if content == "" {
		return "", 0
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)
	start := offset - 1
	if start < 0 {
		start = 0
	}
	if start >= total {
		return "", total
	}
	end := total
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return strings.Join(lines[start:end], ""), total
}
