package toolapproval

import (
	"os"
	"path/filepath"
	"strings"
)

type pathBoundary struct {
	workspace    string
	cwd          string
	allowedFiles map[string]struct{}
}

func newPathBoundary(workspace, cwd string) (pathBoundary, bool) {
	return newPathBoundaryWithAllowedFiles(workspace, cwd, nil)
}

func newPathBoundaryWithAllowedFiles(workspace, cwd string, allowedFiles []string) (pathBoundary, bool) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return pathBoundary{}, false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return pathBoundary{}, false
	}
	current := root
	if strings.TrimSpace(cwd) != "" {
		current = filepath.Join(root, filepath.FromSlash(cwd))
	}
	current, err = filepath.Abs(current)
	if err != nil || !withinRoot(root, current) {
		return pathBoundary{}, false
	}
	allowed := make(map[string]struct{}, len(allowedFiles))
	for _, value := range allowedFiles {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		canonical, evalErr := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(value)))
		if evalErr != nil {
			continue
		}
		info, statErr := os.Stat(canonical)
		if statErr == nil && !info.IsDir() {
			allowed[filepath.Clean(canonical)] = struct{}{}
		}
	}
	return pathBoundary{workspace: root, cwd: current, allowedFiles: allowed}, true
}

func (boundary pathBoundary) changeDirectory(args []string) (pathBoundary, bool) {
	operands := make([]string, 0, 1)
	flagsEnded := false
	for _, arg := range args {
		if !flagsEnded && arg == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && (arg == "-L" || arg == "-P") {
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "-") {
			return pathBoundary{}, false
		}
		operands = append(operands, arg)
	}
	if len(operands) != 1 || strings.TrimSpace(operands[0]) == "" || operands[0] == "-" {
		return pathBoundary{}, false
	}
	candidate := filepath.FromSlash(operands[0])
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(boundary.cwd, candidate)
	}
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil || !withinRoot(boundary.workspace, canonical) {
		return pathBoundary{}, false
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return pathBoundary{}, false
	}
	return pathBoundary{workspace: boundary.workspace, cwd: canonical, allowedFiles: boundary.allowedFiles}, true
}

func (boundary pathBoundary) containsLiteral(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return true
	}
	if strings.HasPrefix(value, "~") || strings.ContainsAny(value, "*?[\x00\r\n") {
		return false
	}
	candidate := filepath.FromSlash(value)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(boundary.cwd, candidate)
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	if !withinRoot(boundary.workspace, candidate) {
		canonical, evalErr := filepath.EvalSymlinks(candidate)
		if evalErr != nil {
			return false
		}
		_, allowed := boundary.allowedFiles[filepath.Clean(canonical)]
		return allowed
	}
	// Lexical containment is insufficient when an existing workspace symlink
	// points outside. For a path that does not exist yet, canonicalize its
	// nearest existing ancestor so writes through such a symlink still prompt.
	existing := candidate
	for {
		if _, statErr := os.Lstat(existing); statErr == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return false
		}
		existing = parent
	}
	canonical, err := filepath.EvalSymlinks(existing)
	return err == nil && withinRoot(boundary.workspace, canonical)
}

func (boundary pathBoundary) isWorkspaceRoot(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "~") || strings.ContainsAny(value, "*?[\x00\r\n") {
		return false
	}
	candidate := filepath.FromSlash(value)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(boundary.cwd, candidate)
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	if canonical, evalErr := filepath.EvalSymlinks(candidate); evalErr == nil {
		candidate = canonical
	}
	return filepath.Clean(candidate) == filepath.Clean(boundary.workspace)
}

func withinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func nonFlagArguments(args []string) []string {
	result := make([]string, 0, len(args))
	flagsEnded := false
	for _, arg := range args {
		if !flagsEnded && arg == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "-") && arg != "-" {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func allPathsInside(boundary pathBoundary, paths []string) bool {
	for _, path := range paths {
		if !boundary.containsLiteral(path) {
			return false
		}
	}
	return true
}
