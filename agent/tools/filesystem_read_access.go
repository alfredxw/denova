package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FilesystemScope classifies a resolved local path against the Project root.
// The Project remains the default base path, while authorization for external
// paths is owned by the host Permission Policy.
type FilesystemScope string

const (
	FilesystemScopeProject  FilesystemScope = "project"
	FilesystemScopeExternal FilesystemScope = "external"
)

// FilesystemReadGrant is the smallest stable path boundary that can authorize
// one local read. Recursive grants cover the directory itself and descendants;
// non-recursive grants cover exactly one file.
type FilesystemReadGrant struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// FilesystemReadPlan is the provider-neutral access projection shared by the
// local read/search executors and Denova's Permission Policy.
type FilesystemReadPlan struct {
	External []FilesystemReadGrant `json:"external,omitempty"`
}

type resolvedReadPath struct {
	absolute string
	display  string
	relative string
	scope    FilesystemScope
	info     os.FileInfo
}

func (workspace *LocalWorkspace) resolveReadPath(input string, allowProjectRoot bool) (resolvedReadPath, error) {
	if workspace == nil || workspace.root == "" {
		return resolvedReadPath{}, errors.New("filesystem Project root is not configured")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return resolvedReadPath{}, errors.New("filesystem path is required")
	}
	if hasResourceScheme(input) {
		return resolvedReadPath{}, fmt.Errorf("resource URI is not a local filesystem path: %s", input)
	}
	native := filepath.FromSlash(input)
	if isAbsoluteFilesystemPath(input) && !filepath.IsAbs(native) {
		return resolvedReadPath{}, fmt.Errorf("absolute path is not valid on this host: %s", input)
	}
	absolute := native
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(workspace.root, absolute)
	}
	absolute = filepath.Clean(absolute)
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if relative, inside := relativeInside(workspace.root, absolute); inside {
			return resolvedReadPath{}, fmt.Errorf("inspect workspace path %s: %w", filepath.ToSlash(relative), err)
		}
		return resolvedReadPath{}, fmt.Errorf("resolve filesystem path %s: %w", filepath.ToSlash(absolute), err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return resolvedReadPath{}, fmt.Errorf("inspect filesystem path %s: %w", filepath.ToSlash(canonical), err)
	}
	canonical = filepath.Clean(canonical)
	relative, inside := relativeInside(workspace.root, canonical)
	if inside {
		if relative == "." && !allowProjectRoot {
			return resolvedReadPath{}, errors.New("path must identify an item inside the active Project")
		}
		if err := workspace.verifyRootIdentity(); err != nil {
			return resolvedReadPath{}, err
		}
		return resolvedReadPath{
			absolute: canonical,
			display:  filepath.ToSlash(relative),
			relative: filepath.ToSlash(relative),
			scope:    FilesystemScopeProject,
			info:     info,
		}, nil
	}
	return resolvedReadPath{
		absolute: canonical,
		display:  filepath.ToSlash(canonical),
		scope:    FilesystemScopeExternal,
		info:     info,
	}, nil
}

func relativeInside(root, target string) (string, bool) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

func isAbsoluteFilesystemPath(value string) bool {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(filepath.FromSlash(value)) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func readGrant(target resolvedReadPath) FilesystemReadGrant {
	return FilesystemReadGrant{Path: filepath.ToSlash(target.absolute), Recursive: target.info.IsDir()}
}

func normalizeFilesystemReadPlan(plan FilesystemReadPlan) FilesystemReadPlan {
	if len(plan.External) == 0 {
		return FilesystemReadPlan{}
	}
	grants := make([]FilesystemReadGrant, 0, len(plan.External))
	for _, grant := range plan.External {
		path := strings.TrimSpace(grant.Path)
		if path == "" {
			continue
		}
		grant.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		grants = append(grants, grant)
	}
	sort.Slice(grants, func(left, right int) bool {
		if grants[left].Path == grants[right].Path {
			return grants[left].Recursive && !grants[right].Recursive
		}
		return grants[left].Path < grants[right].Path
	})
	result := grants[:0]
	for _, grant := range grants {
		if len(result) > 0 && result[len(result)-1].Path == grant.Path {
			result[len(result)-1].Recursive = result[len(result)-1].Recursive || grant.Recursive
			continue
		}
		result = append(result, grant)
	}
	return FilesystemReadPlan{External: append([]FilesystemReadGrant(nil), result...)}
}

// FilesystemReadGrantContains reports whether a remembered grant covers a
// requested external filesystem target. Both sides must already be absolute.
func FilesystemReadGrantContains(grant, requested FilesystemReadGrant) bool {
	grantPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(grant.Path)))
	requestedPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(requested.Path)))
	if grantPath == "." || requestedPath == "." || !filepath.IsAbs(grantPath) || !filepath.IsAbs(requestedPath) {
		return false
	}
	if !grant.Recursive {
		return !requested.Recursive && sameFilesystemPath(grantPath, requestedPath)
	}
	_, inside := relativeInside(grantPath, requestedPath)
	return inside
}

func sameFilesystemPath(left, right string) bool {
	relative, err := filepath.Rel(filepath.Clean(left), filepath.Clean(right))
	return err == nil && relative == "."
}

// PlanFilesystemRead parses one built-in read/glob/grep call into external
// filesystem grants. The boolean is false for non-filesystem read resources
// and unrelated tools. Invalid filesystem calls return the same validation
// error that execution will return, so permission never guesses at a target.
func PlanFilesystemRead(projectRoot, toolName, arguments string) (FilesystemReadPlan, bool, error) {
	workspace, err := OpenWorkspace(projectRoot)
	if err != nil {
		return FilesystemReadPlan{}, true, err
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return FilesystemReadPlan{}, true, fmt.Errorf("decode read access: %w", err)
		}
		if hasResourceScheme(input.Path) {
			return FilesystemReadPlan{}, false, nil
		}
		target, err := workspace.resolveReadPath(input.Path, true)
		if err != nil {
			return FilesystemReadPlan{}, true, err
		}
		if target.scope == FilesystemScopeExternal {
			return FilesystemReadPlan{External: []FilesystemReadGrant{readGrant(target)}}, true, nil
		}
		return FilesystemReadPlan{}, true, nil
	case "glob":
		var input globInput
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return FilesystemReadPlan{}, true, fmt.Errorf("decode glob access: %w", err)
		}
		_, _, plan, err := workspace.globDomains(normalizeRequestedPaths(input.Paths))
		return normalizeFilesystemReadPlan(plan), true, err
	case "grep":
		var input grepInput
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return FilesystemReadPlan{}, true, fmt.Errorf("decode grep access: %w", err)
		}
		command, err := workspace.compileGrepCommand(input.Command)
		if err != nil {
			return FilesystemReadPlan{}, true, err
		}
		return normalizeFilesystemReadPlan(command.access), true, nil
	default:
		return FilesystemReadPlan{}, false, nil
	}
}
