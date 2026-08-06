package tools

import (
	"errors"
	"path/filepath"
	"strconv"
)

// grepCommandPolicyVersion invalidates cursors whenever parsing, safety, or
// canonical output semantics change.
const grepCommandPolicyVersion = 3

type grepOutputMode uint8

const (
	grepOutputContent grepOutputMode = iota
	grepOutputFiles
	grepOutputCount
)

type grepFlagEffect uint8

const (
	grepFlagNone grepFlagEffect = iota
	grepFlagRegexp
	grepFlagBeforeContext
	grepFlagAfterContext
	grepFlagContext
	grepFlagFilesWithMatches
	grepFlagFilesWithoutMatch
	grepFlagCount
	grepFlagCountMatches
)

type grepFlagSpec struct {
	value    bool
	effect   grepFlagEffect
	validate func(string) error
}

type compiledGrepCommand struct {
	args      []string
	paths     []string
	warnings  []string
	mode      grepOutputMode
	modeFlag  string
	hasRegexp bool

	contextBefore int
	contextAfter  int
}

func (command compiledGrepCommand) groupsContext() bool {
	return command.mode == grepOutputContent && (command.contextBefore > 0 || command.contextAfter > 0)
}

func noValueGrepFlag(effect grepFlagEffect) grepFlagSpec {
	return grepFlagSpec{effect: effect}
}

func valueGrepFlag(effect grepFlagEffect, validate func(string) error) grepFlagSpec {
	return grepFlagSpec{value: true, effect: effect, validate: validate}
}

var grepLongFlagSpecs = map[string]grepFlagSpec{
	// Pattern and matching semantics.
	"regexp":              valueGrepFlag(grepFlagRegexp, nil),
	"case-sensitive":      noValueGrepFlag(grepFlagNone),
	"crlf":                noValueGrepFlag(grepFlagNone),
	"encoding":            valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"engine":              valueGrepFlag(grepFlagNone, grepEngineValue),
	"fixed-strings":       noValueGrepFlag(grepFlagNone),
	"ignore-case":         noValueGrepFlag(grepFlagNone),
	"invert-match":        noValueGrepFlag(grepFlagNone),
	"line-regexp":         noValueGrepFlag(grepFlagNone),
	"max-count":           valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	"multiline":           noValueGrepFlag(grepFlagNone),
	"multiline-dotall":    noValueGrepFlag(grepFlagNone),
	"no-crlf":             noValueGrepFlag(grepFlagNone),
	"no-fixed-strings":    noValueGrepFlag(grepFlagNone),
	"no-invert-match":     noValueGrepFlag(grepFlagNone),
	"no-line-regexp":      noValueGrepFlag(grepFlagNone),
	"no-multiline":        noValueGrepFlag(grepFlagNone),
	"no-multiline-dotall": noValueGrepFlag(grepFlagNone),
	"no-pcre2":            noValueGrepFlag(grepFlagNone),
	"no-text":             noValueGrepFlag(grepFlagNone),
	"no-unicode":          noValueGrepFlag(grepFlagNone),
	"no-word-regexp":      noValueGrepFlag(grepFlagNone),
	"pcre2":               noValueGrepFlag(grepFlagNone),
	"smart-case":          noValueGrepFlag(grepFlagNone),
	"stop-on-nonmatch":    noValueGrepFlag(grepFlagNone),
	"text":                noValueGrepFlag(grepFlagNone),
	"unicode":             noValueGrepFlag(grepFlagNone),
	"word-regexp":         noValueGrepFlag(grepFlagNone),

	// Workspace-local traversal and filtering semantics.
	"binary":                   noValueGrepFlag(grepFlagNone),
	"glob":                     valueGrepFlag(grepFlagNone, nil),
	"glob-case-insensitive":    noValueGrepFlag(grepFlagNone),
	"hidden":                   noValueGrepFlag(grepFlagNone),
	"iglob":                    valueGrepFlag(grepFlagNone, nil),
	"ignore":                   noValueGrepFlag(grepFlagNone),
	"ignore-dot":               noValueGrepFlag(grepFlagNone),
	"ignore-exclude":           noValueGrepFlag(grepFlagNone),
	"ignore-files":             noValueGrepFlag(grepFlagNone),
	"ignore-vcs":               noValueGrepFlag(grepFlagNone),
	"max-depth":                valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	"max-filesize":             valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"no-binary":                noValueGrepFlag(grepFlagNone),
	"no-glob-case-insensitive": noValueGrepFlag(grepFlagNone),
	"no-hidden":                noValueGrepFlag(grepFlagNone),
	"no-ignore":                noValueGrepFlag(grepFlagNone),
	"no-ignore-dot":            noValueGrepFlag(grepFlagNone),
	"no-ignore-exclude":        noValueGrepFlag(grepFlagNone),
	"no-ignore-files":          noValueGrepFlag(grepFlagNone),
	"no-ignore-vcs":            noValueGrepFlag(grepFlagNone),
	"no-one-file-system":       noValueGrepFlag(grepFlagNone),
	"one-file-system":          noValueGrepFlag(grepFlagNone),
	"type":                     valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"type-add":                 valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"type-clear":               valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"type-not":                 valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	"unrestricted":             noValueGrepFlag(grepFlagNone),

	// Output semantics whose framing remains compatible with bounded records.
	"after-context":          valueGrepFlag(grepFlagAfterContext, nonNegativeGrepInteger),
	"before-context":         valueGrepFlag(grepFlagBeforeContext, nonNegativeGrepInteger),
	"column":                 noValueGrepFlag(grepFlagNone),
	"context":                valueGrepFlag(grepFlagContext, nonNegativeGrepInteger),
	"count":                  noValueGrepFlag(grepFlagCount),
	"count-matches":          noValueGrepFlag(grepFlagCountMatches),
	"files-with-matches":     noValueGrepFlag(grepFlagFilesWithMatches),
	"files-without-match":    noValueGrepFlag(grepFlagFilesWithoutMatch),
	"include-zero":           noValueGrepFlag(grepFlagNone),
	"line-number":            noValueGrepFlag(grepFlagNone),
	"max-columns":            valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	"max-columns-preview":    noValueGrepFlag(grepFlagNone),
	"no-column":              noValueGrepFlag(grepFlagNone),
	"no-max-columns-preview": noValueGrepFlag(grepFlagNone),
	"only-matching":          noValueGrepFlag(grepFlagNone),
	"with-filename":          noValueGrepFlag(grepFlagNone),

	// Harmless repetitions of Denova-owned invariants are accepted so a normal
	// rg command remains copyable. The implementation appends the canonical
	// value after all model-controlled options.
	"color":            valueGrepFlag(grepFlagNone, grepNeverColorValue),
	"no-config":        noValueGrepFlag(grepFlagNone),
	"no-follow":        noValueGrepFlag(grepFlagNone),
	"no-heading":       noValueGrepFlag(grepFlagNone),
	"no-ignore-global": noValueGrepFlag(grepFlagNone),
	"no-ignore-parent": noValueGrepFlag(grepFlagNone),
	"no-messages":      noValueGrepFlag(grepFlagNone),
	"no-pre":           noValueGrepFlag(grepFlagNone),
	"no-require-git":   noValueGrepFlag(grepFlagNone),
	"no-search-zip":    noValueGrepFlag(grepFlagNone),
	"no-stats":         noValueGrepFlag(grepFlagNone),
	"path-separator":   valueGrepFlag(grepFlagNone, grepSlashSeparatorValue),
	"sort":             valueGrepFlag(grepFlagNone, grepPathSortValue),
}

var grepShortFlagSpecs = map[byte]grepFlagSpec{
	'.': noValueGrepFlag(grepFlagNone), // --hidden
	'A': valueGrepFlag(grepFlagAfterContext, nonNegativeGrepInteger),
	'B': valueGrepFlag(grepFlagBeforeContext, nonNegativeGrepInteger),
	'C': valueGrepFlag(grepFlagContext, nonNegativeGrepInteger),
	'E': valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	'F': noValueGrepFlag(grepFlagNone),
	'H': noValueGrepFlag(grepFlagNone),
	'M': valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	'P': noValueGrepFlag(grepFlagNone),
	'S': noValueGrepFlag(grepFlagNone),
	'T': valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	'U': noValueGrepFlag(grepFlagNone),
	'a': noValueGrepFlag(grepFlagNone),
	'c': noValueGrepFlag(grepFlagCount),
	'd': valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	'e': valueGrepFlag(grepFlagRegexp, nil),
	'g': valueGrepFlag(grepFlagNone, nil),
	'i': noValueGrepFlag(grepFlagNone),
	'l': noValueGrepFlag(grepFlagFilesWithMatches),
	'm': valueGrepFlag(grepFlagNone, nonNegativeGrepInteger),
	'n': noValueGrepFlag(grepFlagNone),
	'o': noValueGrepFlag(grepFlagNone),
	's': noValueGrepFlag(grepFlagNone),
	't': valueGrepFlag(grepFlagNone, nonEmptyGrepFlagValue),
	'u': noValueGrepFlag(grepFlagNone),
	'v': noValueGrepFlag(grepFlagNone),
	'w': noValueGrepFlag(grepFlagNone),
	'x': noValueGrepFlag(grepFlagNone),
}

var grepUnsafeLongFlags = map[string]string{
	"config":                  "external ripgrep configuration is disabled",
	"context-separator":       "result framing is owned by Denova",
	"colors":                  "result coloring is owned by Denova",
	"debug":                   "debug output breaks the stable result contract",
	"dfa-size-limit":          "regex memory policy is owned by Denova",
	"field-context-separator": "result framing is owned by Denova",
	"field-match-separator":   "result framing is owned by Denova",
	"file":                    "pattern files are not passed directly to ripgrep; use repeated -e",
	"files":                   "path discovery belongs to glob",
	"generate":                "non-search generation modes are unsupported",
	"heading":                 "result framing is owned by Denova",
	"help":                    "non-search help output is unsupported",
	"hostname-bin":            "this flag can start an external program",
	"hyperlink-format":        "result hyperlinks are disabled",
	"ignore-file":             "external ignore files are not passed directly to ripgrep; use -g",
	"ignore-global":           "ignore files outside the workspace are disabled",
	"ignore-parent":           "ignore files outside the workspace are disabled",
	"json":                    "machine output is owned by Denova",
	"mmap":                    "process resource policy is owned by Denova",
	"no-filename":             "stable results require workspace paths",
	"no-line-number":          "stable content results require source line numbers",
	"null":                    "NUL-delimited output is incompatible with result paging",
	"null-data":               "NUL-delimited output is incompatible with result paging",
	"passthru":                "printing non-matching workspace content is unsupported",
	"pre":                     "this flag can start an external program",
	"pre-glob":                "preprocessors are disabled",
	"pretty":                  "pretty output breaks the stable result contract",
	"quiet":                   "quiet mode suppresses recoverable results",
	"regex-size-limit":        "regex memory policy is owned by Denova",
	"replace":                 "transformed output is not safe for source recovery",
	"require-git":             "workspace ignore semantics are owned by Denova",
	"search-zip":              "this flag can start external decompression programs",
	"sort-files":              "result ordering is owned by Denova",
	"sortr":                   "result ordering is owned by Denova",
	"stats":                   "statistics break the stable result contract",
	"threads":                 "process resource policy is owned by Denova",
	"trace":                   "trace output breaks the stable result contract",
	"trim":                    "trimmed output is not safe for source recovery",
	"type-list":               "non-search type listing is unsupported",
	"version":                 "non-search version output is unsupported",
	"vimgrep":                 "result framing is owned by Denova",
	"follow":                  "following symlinks may leave the workspace",
}

var grepUnsafeShortFlags = map[byte]string{
	'0': "NUL-delimited output is incompatible with result paging",
	'I': "stable results require workspace paths",
	'L': "following symlinks may leave the workspace",
	'N': "stable content results require source line numbers",
	'V': "non-search version output is unsupported",
	'b': "result framing is owned by Denova",
	'f': "pattern files are not passed directly to ripgrep; use repeated -e",
	'h': "non-search help output is unsupported",
	'j': "process resource policy is owned by Denova",
	'p': "pretty output breaks the stable result contract",
	'q': "quiet mode suppresses recoverable results",
	'r': "transformed output is not safe for source recovery",
	'z': "this flag can start external decompression programs",
}

func grepArguments(command compiledGrepCommand) []string {
	args := append([]string(nil), command.args...)
	args = append(args,
		"--no-config", "--color=never", "--no-messages", "--no-require-git",
		"--no-ignore-global", "--no-ignore-parent", "--no-follow", "--no-heading",
		"--no-pre", "--no-search-zip", "--no-stats", "--sort=path",
		"--path-separator=/", "--glob=!.git/**",
	)
	switch command.mode {
	case grepOutputCount:
		args = append(args, "--with-filename")
	case grepOutputFiles:
	default:
		args = append(args, "--with-filename", "--line-number")
		if command.groupsContext() {
			args = append(args, "--context-separator=--")
		}
	}
	args = append(args, "--")
	for _, target := range command.paths {
		args = append(args, filepath.FromSlash(target))
	}
	return args
}

func nonNegativeGrepInteger(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return errors.New("expected a non-negative integer")
	}
	return nil
}

func nonEmptyGrepFlagValue(value string) error {
	if value == "" {
		return errors.New("expected a non-empty value")
	}
	return nil
}

func grepEngineValue(value string) error {
	switch value {
	case "default", "pcre2", "auto":
		return nil
	default:
		return errors.New("expected default, pcre2, or auto")
	}
}

func grepNeverColorValue(value string) error {
	if value != "never" {
		return errors.New("Denova only accepts --color=never")
	}
	return nil
}

func grepPathSortValue(value string) error {
	if value != "path" {
		return errors.New("Denova only accepts --sort=path")
	}
	return nil
}

func grepSlashSeparatorValue(value string) error {
	if value != "/" {
		return errors.New("Denova only accepts --path-separator=/")
	}
	return nil
}
