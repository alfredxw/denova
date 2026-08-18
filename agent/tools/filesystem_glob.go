package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

type globTargetKind uint8

const (
	globTargetPattern globTargetKind = iota
	globTargetFile
	globTargetDirectory
)

type globTarget struct {
	kind  globTargetKind
	value string
	grant *FilesystemReadGrant
}

type globDomain struct {
	root     string
	identity os.FileInfo
	project  bool
	targets  []globTarget
}

// Glob discovers de-duplicated files and directories without following
// directory symlinks. Ripgrep supplies mature hidden/.gitignore semantics;
// doublestar is used only for matching the already-safe relative candidates.
func (workspace *LocalWorkspace) Glob(ctx context.Context, request GlobRequest) (SearchResult, error) {
	if workspace == nil || workspace.Root() == "" {
		return SearchResult{}, errors.New("workspace is not configured")
	}
	limits := workspace.Limits()
	request = normalizeGlobRequest(request)
	limit := request.Limit
	if limit <= 0 {
		limit = limits.DefaultDirectoryItems
	}
	after, err := decodeGlobCursor(request.Cursor, request)
	if err != nil {
		return SearchResult{}, err
	}
	domains, warnings, _, err := workspace.globDomains(request.Paths)
	if err != nil {
		return SearchResult{}, err
	}

	capacity := min(limit, limits.MaxResultBytes)
	if capacity < limits.MaxResultBytes {
		capacity++
	}
	capacity = min(capacity, maxFilesystemScanEntries)
	matches := make([]string, 0, min(capacity, limits.DefaultDirectoryItems))
	seenDirectories := make(map[string]struct{})
	eligible := 0
	add := func(candidate string) bool {
		if strings.HasSuffix(candidate, "/") {
			if _, exists := seenDirectories[candidate]; exists {
				return true
			}
			seenDirectories[candidate] = struct{}{}
		}
		if candidate <= after {
			return true
		}
		eligible++
		index := sort.SearchStrings(matches, candidate)
		if index < len(matches) && matches[index] == candidate {
			eligible--
			return true
		}
		if index >= capacity {
			return true
		}
		matches = append(matches, "")
		copy(matches[index+1:], matches[index:])
		matches[index] = candidate
		if len(matches) > capacity {
			matches = matches[:capacity]
		}
		return true
	}

	for _, domain := range domains {
		if !domain.project {
			for _, target := range domain.targets {
				if target.kind == globTargetDirectory && target.value == "." {
					add(domain.display("./"))
					break
				}
			}
		}
		if err := workspace.scanGlobFiles(ctx, request, domain, limits.MaxResultBytes, add); err != nil {
			return SearchResult{}, err
		}
		stoppedEarly, err := workspace.addGlobDirectories(ctx, request, domain, add)
		if err != nil {
			return SearchResult{}, err
		}
		if stoppedEarly {
			return SearchResult{}, errors.New("glob directory scan stopped before producing a stable page")
		}
	}
	visible := min(limit, len(matches))
	outputBytes := 0
	for index := 0; index < visible; index++ {
		additional := len(matches[index])
		if index > 0 {
			additional++
		}
		if outputBytes+additional > limits.MaxResultBytes {
			visible = index
			break
		}
		outputBytes += additional
	}
	if visible == 0 && len(matches) > 0 {
		return SearchResult{}, fmt.Errorf("glob entry exceeds the %d-byte shared result budget", limits.MaxResultBytes)
	}
	result := SearchResult{Entries: append([]string(nil), matches[:visible]...), Truncated: eligible > visible, Warnings: warnings}
	if result.Truncated && visible > 0 {
		result.NextCursor, err = encodeGlobCursor(result.Entries[visible-1], request)
		if err != nil {
			return SearchResult{}, fmt.Errorf("encode glob cursor: %w", err)
		}
	}
	return result, nil
}

func (workspace *LocalWorkspace) scanGlobFiles(
	ctx context.Context,
	request GlobRequest,
	domain globDomain,
	maxResultBytes int,
	add func(string) bool,
) error {
	args := []string{
		"--files", "--null", "--no-config", "--no-messages", "--no-require-git",
		"--no-ignore-global", "--no-ignore-parent", "--sort=path", "--glob", "!.git/**",
	}
	if request.Hidden {
		args = append(args, "--hidden")
	}
	if !request.Gitignore {
		args = append(args, "--no-ignore")
	}
	command := exec.CommandContext(ctx, workspace.ripgrepExecutable, args...)
	command.Dir = domain.root
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create glob stdout for %s: %w", filepath.ToSlash(domain.root), err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("create glob stderr for %s: %w", filepath.ToSlash(domain.root), err)
	}
	if err := verifyFilesystemRoot(domain.root, domain.identity); err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start filesystem glob at %s: %w", filepath.ToSlash(domain.root), err)
	}
	diagnostics := readProcessDiagnostics(stderr, maxResultBytes)
	reader := bufio.NewReaderSize(stdout, 64*1024)
	var loopErr error
	for {
		if err := contextError(ctx); err != nil {
			_ = command.Process.Kill()
			loopErr = err
			break
		}
		record, readErr := reader.ReadString(0)
		if len(record) > 0 {
			record = strings.TrimSuffix(record, "\x00")
			candidate := filepath.ToSlash(record)
			if !utf8.ValidString(candidate) {
				_ = command.Process.Kill()
				loopErr = errors.New("glob encountered a non-UTF-8 filesystem path")
				break
			}
			for _, entry := range globCandidatePaths(candidate) {
				if matchesGlobTargets(entry, domain.targets) {
					add(domain.display(entry))
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = command.Process.Kill()
				loopErr = fmt.Errorf("read glob output: %w", readErr)
			}
			break
		}
	}
	diagnostic := <-diagnostics
	waitErr := command.Wait()
	if loopErr != nil {
		return loopErr
	}
	if diagnostic.err != nil {
		return diagnostic.err
	}
	if waitErr != nil {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 1 {
			return fmt.Errorf("filesystem glob failed at %s: %s", filepath.ToSlash(domain.root), boundedString(diagnostic.content, maxResultBytes))
		}
	}
	return nil
}

func normalizeGlobRequest(request GlobRequest) GlobRequest {
	request.Paths = normalizeRequestedPaths(request.Paths)
	return request
}

func normalizeRequestedPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	result := make([]string, len(paths))
	for index, value := range paths {
		result[index] = filepath.ToSlash(strings.TrimSpace(value))
	}
	return result
}

func (workspace *LocalWorkspace) globDomains(inputs []string) ([]globDomain, []string, FilesystemReadPlan, error) {
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	domains := make([]globDomain, 0, len(inputs))
	warnings := make([]string, 0)
	plan := FilesystemReadPlan{}
	for _, input := range inputs {
		input = filepath.ToSlash(strings.TrimSpace(input))
		if input == "" {
			return nil, nil, FilesystemReadPlan{}, errors.New("glob paths must not contain an empty entry")
		}
		if hasResourceScheme(input) {
			return nil, nil, FilesystemReadPlan{}, fmt.Errorf("glob only supports local filesystem paths: %s", input)
		}
		if hasGlobMeta(input) {
			domain, target, err := workspace.globPatternDomain(input)
			if err != nil {
				if len(inputs) == 1 {
					return nil, nil, FilesystemReadPlan{}, err
				}
				warnings = append(warnings, fmt.Sprintf("Skipped invalid glob pattern %q: %v.", input, err))
				continue
			}
			domains = appendGlobDomain(domains, domain, target)
			if target.grant != nil {
				plan.External = append(plan.External, *target.grant)
			}
			continue
		}
		resolved, err := workspace.resolveReadPath(input, true)
		if err != nil {
			if len(inputs) == 1 {
				return nil, nil, FilesystemReadPlan{}, err
			}
			warnings = append(warnings, fmt.Sprintf("Skipped missing path %q.", input))
			continue
		}
		kind := globTargetFile
		if resolved.info.IsDir() {
			kind = globTargetDirectory
		}
		domain := globDomain{root: workspace.root, identity: workspace.rootIdentity, project: true}
		target := globTarget{kind: kind, value: resolved.relative}
		if resolved.scope == FilesystemScopeExternal {
			targetGrant := readGrant(resolved)
			target.grant = &targetGrant
			plan.External = append(plan.External, targetGrant)
			if resolved.info.IsDir() {
				domain = globDomain{root: resolved.absolute, identity: resolved.info}
				target.value = "."
			} else {
				parent := filepath.Dir(resolved.absolute)
				parentInfo, statErr := os.Stat(parent)
				if statErr != nil {
					return nil, nil, FilesystemReadPlan{}, fmt.Errorf("inspect glob parent %s: %w", filepath.ToSlash(parent), statErr)
				}
				domain = globDomain{root: parent, identity: parentInfo}
				target.value = filepath.ToSlash(filepath.Base(resolved.absolute))
			}
		}
		domains = appendGlobDomain(domains, domain, target)
	}
	if len(domains) == 0 {
		return nil, nil, FilesystemReadPlan{}, errors.New("none of the requested glob paths is searchable")
	}
	return domains, warnings, normalizeFilesystemReadPlan(plan), nil
}

func (workspace *LocalWorkspace) globPatternDomain(input string) (globDomain, globTarget, error) {
	value := filepath.ToSlash(strings.TrimSpace(input))
	meta := strings.IndexAny(value, "*?[")
	if brace := strings.Index(value, "{"); meta < 0 || (brace >= 0 && brace < meta) {
		meta = brace
	}
	if meta < 0 {
		return globDomain{}, globTarget{}, fmt.Errorf("glob pattern has no wildcard syntax: %s", input)
	}
	separator := strings.LastIndex(value[:meta], "/")
	anchor, pattern := ".", value
	if separator >= 0 {
		anchor = value[:separator]
		pattern = value[separator+1:]
		if anchor == "" {
			anchor = "/"
		} else if len(anchor) == 2 && anchor[1] == ':' {
			anchor += "/"
		}
	}
	if hasParentComponent(pattern) || !doublestar.ValidatePathPattern(pattern) {
		return globDomain{}, globTarget{}, fmt.Errorf("glob pattern is invalid: %s", input)
	}
	resolved, err := workspace.resolveReadPath(anchor, true)
	if err != nil {
		return globDomain{}, globTarget{}, fmt.Errorf("resolve glob root %s: %w", anchor, err)
	}
	if !resolved.info.IsDir() {
		return globDomain{}, globTarget{}, fmt.Errorf("glob root is not a directory: %s", resolved.display)
	}
	if resolved.scope == FilesystemScopeProject {
		value = pattern
		if resolved.relative != "." {
			value = path.Join(resolved.relative, pattern)
		}
		if !doublestar.ValidatePathPattern(value) {
			return globDomain{}, globTarget{}, fmt.Errorf("glob pattern is invalid: %s", input)
		}
		return globDomain{root: workspace.root, identity: workspace.rootIdentity, project: true}, globTarget{
			kind: globTargetPattern, value: value,
		}, nil
	}
	grant := FilesystemReadGrant{Path: filepath.ToSlash(resolved.absolute), Recursive: true}
	return globDomain{root: resolved.absolute, identity: resolved.info}, globTarget{
		kind: globTargetPattern, value: path.Clean(pattern), grant: &grant,
	}, nil
}

func appendGlobDomain(domains []globDomain, domain globDomain, target globTarget) []globDomain {
	for index := range domains {
		if domains[index].project == domain.project && sameFilesystemPath(domains[index].root, domain.root) {
			domains[index].targets = append(domains[index].targets, target)
			return domains
		}
	}
	domain.targets = []globTarget{target}
	return append(domains, domain)
}

func (domain globDomain) display(relative string) string {
	if domain.project {
		return relative
	}
	directory := strings.HasSuffix(relative, "/")
	value := filepath.ToSlash(filepath.Join(domain.root, filepath.FromSlash(strings.TrimSuffix(relative, "/"))))
	if directory {
		value += "/"
	}
	return value
}

func verifyFilesystemRoot(root string, identity os.FileInfo) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("verify filesystem root %s: %w", filepath.ToSlash(root), err)
	}
	if identity == nil || !os.SameFile(identity, info) {
		return fmt.Errorf("filesystem root identity changed; resolve the path again: %s", filepath.ToSlash(root))
	}
	return nil
}

func matchesGlobTargets(candidate string, targets []globTarget) bool {
	plain := strings.TrimSuffix(candidate, "/")
	for _, target := range targets {
		switch target.kind {
		case globTargetFile:
			if plain == target.value {
				return true
			}
		case globTargetDirectory:
			if target.value == "." || strings.HasPrefix(plain, target.value+"/") {
				return true
			}
		case globTargetPattern:
			matched, _ := doublestar.Match(target.value, plain)
			if matched {
				return true
			}
		}
	}
	return false
}

func globCandidatePaths(file string) []string {
	result := []string{file}
	directory := path.Dir(file)
	for directory != "." && directory != "/" {
		result = append(result, directory+"/")
		directory = path.Dir(directory)
	}
	return result
}

func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[") || strings.Contains(value, "{")
}

func hasParentComponent(value string) bool {
	for _, component := range strings.Split(filepath.ToSlash(value), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}
