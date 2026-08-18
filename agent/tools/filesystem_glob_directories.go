package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

var filesystemIgnoreFiles = [...]string{".gitignore", ".ignore", ".rgignore"}

type filesystemIgnoreBudget struct {
	bytes int
	rules int
}

// addGlobDirectories complements ripgrep's file discovery with directories,
// including empty ones. Traversal stays behind os.Root, never follows
// directory symlinks, and uses the same root-local ignore-file boundary
// configured for ripgrep in Glob.
func (workspace *LocalWorkspace) addGlobDirectories(
	ctx context.Context,
	request GlobRequest,
	domain globDomain,
	add func(string) bool,
) (bool, error) {
	if err := verifyFilesystemRoot(domain.root, domain.identity); err != nil {
		return false, err
	}
	root, err := os.OpenRoot(domain.root)
	if err != nil {
		return false, fmt.Errorf("open glob root %s: %w", filepath.ToSlash(domain.root), err)
	}
	defer root.Close()

	patterns := make([]gitignore.Pattern, 0)
	scanBudget := &filesystemScanBudget{}
	ignoreBudget := &filesystemIgnoreBudget{}
	var walk func(string, []string) (bool, error)
	walk = func(directory string, components []string) (bool, error) {
		patternBase := len(patterns)
		defer func() { patterns = patterns[:patternBase] }()
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if request.Gitignore {
			loaded, err := readFilesystemIgnorePatterns(ctx, root, directory, components, ignoreBudget)
			if err != nil {
				return false, err
			}
			patterns = append(patterns, loaded...)
		}
		children, err := readFilesystemDirectory(ctx, root, directory, scanBudget)
		if err != nil {
			return false, fmt.Errorf("enumerate glob directory %s: %w", directory, err)
		}
		matcher := gitignore.NewMatcher(patterns)
		for _, child := range children {
			if err := contextError(ctx); err != nil {
				return false, err
			}
			name := child.Name()
			if !utf8.ValidString(name) {
				return false, fmt.Errorf("glob encountered a non-UTF-8 directory under %s", directory)
			}
			if name == ".git" || (!request.Hidden && strings.HasPrefix(name, ".")) {
				continue
			}
			if child.Type()&fs.ModeSymlink != 0 || !child.IsDir() {
				continue
			}
			childComponents := appendPathComponent(components, name)
			if len(childComponents) > workspace.Limits().MaxDirectoryDepth {
				return false, fmt.Errorf("glob directory traversal exceeds depth %d under %s; narrow the path", workspace.Limits().MaxDirectoryDepth, directory)
			}
			if request.Gitignore && matcher.Match(childComponents, true) {
				continue
			}
			childPath := name
			if directory != "." {
				childPath = path.Join(directory, name)
			}
			candidate := childPath + "/"
			if matchesGlobTargets(candidate, domain.targets) && !add(domain.display(candidate)) {
				return true, nil
			}
			stopped, err := walk(childPath, childComponents)
			if err != nil || stopped {
				return stopped, err
			}
		}
		return false, nil
	}
	return walk(".", nil)
}

func readFilesystemIgnorePatterns(
	ctx context.Context,
	root *os.Root,
	directory string,
	domain []string,
	budget *filesystemIgnoreBudget,
) ([]gitignore.Pattern, error) {
	patterns := make([]gitignore.Pattern, 0)
	for _, name := range filesystemIgnoreFiles {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		ignorePath := name
		if directory != "." {
			ignorePath = path.Join(directory, name)
		}
		info, err := root.Lstat(filepath.FromSlash(ignorePath))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect workspace ignore file %s: %w", ignorePath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("workspace ignore file must be regular: %s", ignorePath)
		}
		file, err := root.Open(filepath.FromSlash(ignorePath))
		if err != nil {
			return nil, fmt.Errorf("open workspace ignore file %s: %w", ignorePath, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), maxFilesystemIgnoreBytes+1)
		var policyErr error
		for scanner.Scan() {
			if err := contextError(ctx); err != nil {
				policyErr = err
				break
			}
			budget.bytes += len(scanner.Bytes()) + 1
			if budget.bytes > maxFilesystemIgnoreBytes {
				policyErr = fmt.Errorf("filesystem ignore files exceed %d bytes", maxFilesystemIgnoreBytes)
				break
			}
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			budget.rules++
			if budget.rules > maxFilesystemIgnoreRules {
				policyErr = fmt.Errorf("filesystem ignore files exceed %d rules", maxFilesystemIgnoreRules)
				break
			}
			patterns = append(patterns, gitignore.ParsePattern(line, domain))
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if policyErr != nil {
			return nil, policyErr
		}
		if scanErr != nil {
			return nil, fmt.Errorf("read workspace ignore file %s: %w", ignorePath, scanErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close workspace ignore file %s: %w", ignorePath, closeErr)
		}
	}
	return patterns, nil
}

func appendPathComponent(components []string, name string) []string {
	result := make([]string, len(components)+1)
	copy(result, components)
	result[len(components)] = name
	return result
}
