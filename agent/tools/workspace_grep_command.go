package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"mvdan.cc/sh/v3/syntax"
)

const (
	maxGrepCommandBytes   = 64 * 1024
	maxGrepPipelineStages = 16
)

// compileGrepCommand turns Denova's literal command language into an argv plan
// before any process starts. A pipeline may contain only literal rg stages, so
// execution can connect processes directly without granting shell authority.
func (workspace *LocalWorkspace) compileGrepCommand(input string) (compiledGrepCommand, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return compiledGrepCommand{}, grepCommandError("invalid_command", "command is required")
	}
	if len(input) > maxGrepCommandBytes {
		return compiledGrepCommand{}, grepCommandError(
			"invalid_command",
			fmt.Sprintf("command exceeds the %d-byte limit", maxGrepCommandBytes),
		)
	}
	pipeline, err := parseLiteralGrepPipeline(input)
	if err != nil {
		return compiledGrepCommand{}, err
	}
	if len(pipeline) > maxGrepPipelineStages {
		return compiledGrepCommand{}, grepCommandError(
			"invalid_command",
			fmt.Sprintf("grep pipeline exceeds the %d-stage limit", maxGrepPipelineStages),
		)
	}
	command := compiledGrepCommand{stages: make([]compiledGrepStage, 0, len(pipeline))}
	normalized := false
	for index, words := range pipeline {
		stage, warnings, stageNormalized, err := workspace.compileGrepStage(words, index)
		if err != nil {
			return compiledGrepCommand{}, err
		}
		command.stages = append(command.stages, stage)
		command.warnings = append(command.warnings, warnings...)
		normalized = normalized || stageNormalized
	}
	if normalized {
		warning := fmt.Sprintf("Normalized grep command to: %s.", canonicalGrepCommand(command))
		command.warnings = append([]string{warning}, command.warnings...)
	}
	return command, nil
}

// compileGrepStage preserves ripgrep's PATTERN [PATH ...] contract for the
// first stage. Later stages receive only the preceding stdout and therefore
// reject workspace paths instead of silently ignoring the pipeline input.
func (workspace *LocalWorkspace) compileGrepStage(words []string, stageIndex int) (compiledGrepStage, []string, bool, error) {
	words, normalized, err := normalizeGrepWords(words)
	if err != nil {
		return compiledGrepStage{}, nil, false, fmt.Errorf("grep pipeline stage %d: %w", stageIndex+1, err)
	}
	stage := compiledGrepStage{mode: grepOutputContent}
	positionals := make([]string, 0, len(words)-1)
	optionsEnded := false
	for index := 1; index < len(words); index++ {
		token := words[index]
		if !optionsEnded && token == "--" {
			optionsEnded = true
			continue
		}
		if optionsEnded || token == "" || token == "-" || !strings.HasPrefix(token, "-") {
			positionals = append(positionals, token)
			continue
		}
		consumed, parseErr := stage.consumeFlag(words, index)
		if parseErr != nil {
			return compiledGrepStage{}, nil, false, parseErr
		}
		stage.args = append(stage.args, words[index:index+consumed]...)
		index += consumed - 1
	}
	paths := positionals
	if !stage.hasRegexp {
		if len(positionals) == 0 {
			return compiledGrepStage{}, nil, false, grepCommandError("invalid_pattern", "rg command must contain a pattern or -e/--regexp")
		}
		// Canonicalizing the positional pattern through -e also preserves native
		// `rg -- -leading-dash path` behavior after Denova appends its invariants.
		stage.args = append(stage.args, "-e", positionals[0])
		stage.hasRegexp = true
		paths = positionals[1:]
	}
	if stageIndex > 0 {
		if len(paths) > 1 || (len(paths) == 1 && paths[0] != "-") {
			return compiledGrepStage{}, nil, false, grepCommandError(
				"pipeline_path",
				fmt.Sprintf("pipeline stage %d must read only from the preceding rg stage", stageIndex+1),
			)
		}
		stage.paths = []string{"-"}
		return stage, nil, normalized, nil
	}
	validated, globs, warnings, pathsNormalized, err := workspace.validateGrepPaths(paths)
	if err != nil {
		return compiledGrepStage{}, nil, false, err
	}
	for _, glob := range globs {
		stage.args = append(stage.args, "-g", glob)
	}
	stage.paths = validated
	return stage, warnings, normalized || pathsNormalized, nil
}

func (command *compiledGrepStage) consumeFlag(words []string, index int) (int, error) {
	token := words[index]
	if strings.HasPrefix(token, "--") {
		return command.consumeLongFlag(words, index)
	}
	return command.consumeShortFlags(words, index)
}

func (command *compiledGrepStage) consumeLongFlag(words []string, index int) (int, error) {
	token := words[index]
	nameValue := strings.TrimPrefix(token, "--")
	name, value, attached := strings.Cut(nameValue, "=")
	if reason, unsafe := grepUnsafeLongFlags[name]; unsafe {
		return 0, grepCommandError("unsafe_flag", fmt.Sprintf("--%s is unavailable: %s", name, reason))
	}
	spec, ok := grepLongFlagSpecs[name]
	if !ok {
		return 0, grepCommandError("unsupported_flag", fmt.Sprintf("unsupported ripgrep flag --%s", name))
	}
	consumed := 1
	if spec.value {
		if !attached {
			if index+1 >= len(words) || words[index+1] == "--" {
				return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("--%s requires a value", name))
			}
			value = words[index+1]
			consumed = 2
		}
	} else if attached {
		return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("--%s does not accept a value", name))
	}
	if err := command.applyFlag("--"+name, value, spec); err != nil {
		return 0, err
	}
	return consumed, nil
}

func (command *compiledGrepStage) consumeShortFlags(words []string, index int) (int, error) {
	token := words[index]
	body := strings.TrimPrefix(token, "-")
	if body == "" {
		return 0, grepCommandError("invalid_pattern", "use -e '-' to search for a literal dash")
	}
	consumed := 1
	for offset := 0; offset < len(body); offset++ {
		name := body[offset]
		if reason, unsafe := grepUnsafeShortFlags[name]; unsafe {
			return 0, grepCommandError("unsafe_flag", fmt.Sprintf("-%c is unavailable: %s", name, reason))
		}
		spec, ok := grepShortFlagSpecs[name]
		if !ok {
			return 0, grepCommandError("unsupported_flag", fmt.Sprintf("unsupported ripgrep flag -%c", name))
		}
		value := ""
		if spec.value {
			if offset+1 < len(body) {
				value = body[offset+1:]
			} else {
				if index+1 >= len(words) || words[index+1] == "--" {
					return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("-%c requires a value", name))
				}
				value = words[index+1]
				consumed = 2
			}
			if err := command.applyFlag("-"+string(name), value, spec); err != nil {
				return 0, err
			}
			break
		}
		if err := command.applyFlag("-"+string(name), value, spec); err != nil {
			return 0, err
		}
	}
	return consumed, nil
}

func (command *compiledGrepStage) applyFlag(name, value string, spec grepFlagSpec) error {
	if spec.validate != nil {
		if err := spec.validate(value); err != nil {
			return grepCommandError("invalid_flag_value", fmt.Sprintf("%s has invalid value %q: %v", name, value, err))
		}
	}
	switch spec.effect {
	case grepFlagNone:
		return nil
	case grepFlagRegexp:
		command.hasRegexp = true
		return nil
	case grepFlagBeforeContext, grepFlagAfterContext, grepFlagContext:
		parsed, _ := strconv.Atoi(value)
		switch spec.effect {
		case grepFlagBeforeContext:
			command.contextBefore = parsed
		case grepFlagAfterContext:
			command.contextAfter = parsed
		case grepFlagContext:
			command.contextBefore, command.contextAfter = parsed, parsed
		}
		return nil
	case grepFlagFilesWithMatches:
		return command.selectMode(grepOutputFiles, "files-with-matches")
	case grepFlagFilesWithoutMatch:
		return command.selectMode(grepOutputFiles, "files-without-match")
	case grepFlagCount:
		return command.selectMode(grepOutputCount, "count")
	case grepFlagCountMatches:
		return command.selectMode(grepOutputCount, "count-matches")
	default:
		return errors.New("unsupported internal grep flag effect")
	}
}

func (command *compiledGrepStage) selectMode(mode grepOutputMode, flag string) error {
	if command.modeFlag != "" && command.modeFlag != flag {
		return grepCommandError("conflicting_flags", fmt.Sprintf("output modes %s and %s cannot be combined", command.modeFlag, flag))
	}
	command.mode, command.modeFlag = mode, flag
	return nil
}

func (workspace *LocalWorkspace) validateGrepPaths(inputs []string) ([]string, []string, []string, bool, error) {
	if len(inputs) == 0 {
		return []string{"."}, nil, nil, false, nil
	}
	paths := make([]string, 0, len(inputs))
	globs := make([]string, 0)
	warnings := make([]string, 0)
	seenPaths := make(map[string]struct{}, len(inputs))
	seenGlobs := make(map[string]struct{})
	normalized := false
	missing := 0
	for _, input := range inputs {
		plain := filepath.ToSlash(strings.TrimSpace(input))
		if plain == "" || hasResourceScheme(plain) || (isAbsoluteGrepPath(plain) && !filepath.IsAbs(filepath.FromSlash(plain))) {
			return nil, nil, nil, false, grepCommandError("unsafe_path", fmt.Sprintf("grep path is not inside the active workspace: %q", input))
		}
		if filepath.IsAbs(filepath.FromSlash(plain)) {
			if canonical, evalErr := filepath.EvalSymlinks(filepath.FromSlash(plain)); evalErr == nil {
				plain = filepath.ToSlash(canonical)
				normalized = true
			}
		}
		relative, info, err := workspace.stat(plain, true)
		if err != nil {
			if hasGlobMeta(plain) {
				glob, globErr := workspace.normalizeGrepGlob(plain)
				if globErr != nil {
					return nil, nil, nil, false, globErr
				}
				if _, duplicate := seenGlobs[glob]; !duplicate {
					seenGlobs[glob] = struct{}{}
					globs = append(globs, glob)
				}
				normalized = true
				continue
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, nil, false, grepCommandError(
					"unsafe_path",
					fmt.Sprintf("grep path %q cannot be safely inspected: %v", input, err),
				)
			}
			missing++
			warnings = append(warnings, fmt.Sprintf("Skipped missing path %q.", input))
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, nil, nil, false, grepCommandError(
				"unsupported_path",
				fmt.Sprintf("grep path %q must be a regular file or directory", input),
			)
		}
		if _, duplicate := seenPaths[relative]; duplicate {
			continue
		}
		seenPaths[relative] = struct{}{}
		paths = append(paths, relative)
		normalized = normalized || plain != relative
	}
	if len(paths) == 0 && len(globs) > 0 {
		paths = append(paths, ".")
	}
	if len(paths) == 0 {
		if len(inputs) == 1 && missing == 1 {
			return nil, nil, nil, false, grepCommandError("path_not_found", fmt.Sprintf("grep path %q does not exist", inputs[0]))
		}
		return nil, nil, nil, false, grepCommandError("no_searchable_path", "none of the requested grep paths exists")
	}
	sort.Strings(paths)
	return paths, globs, warnings, normalized, nil
}

func (workspace *LocalWorkspace) normalizeGrepGlob(input string) (string, error) {
	value := filepath.ToSlash(strings.TrimSpace(input))
	if hasResourceScheme(value) || (isAbsoluteGrepPath(value) && !filepath.IsAbs(filepath.FromSlash(value))) {
		return "", grepCommandError("unsafe_path", fmt.Sprintf("grep glob is not inside the active workspace: %q", input))
	}
	if filepath.IsAbs(filepath.FromSlash(value)) {
		root := filepath.ToSlash(workspace.root)
		prefix := strings.TrimSuffix(root, "/") + "/"
		if !strings.HasPrefix(value, prefix) {
			return "", grepCommandError("unsafe_path", fmt.Sprintf("grep glob is outside the active workspace: %q", input))
		}
		value = strings.TrimPrefix(value, prefix)
	}
	value = path.Clean(value)
	if value == ".." || strings.HasPrefix(value, "../") || !doublestar.ValidatePathPattern(value) {
		return "", grepCommandError("unsafe_path", fmt.Sprintf("grep glob must resolve inside the active workspace: %q", input))
	}
	return value, nil
}

func isAbsoluteGrepPath(value string) bool {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func parseLiteralGrepPipeline(input string) ([][]string, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(input), "")
	if err != nil {
		return nil, grepCommandError("shell_syntax", fmt.Sprintf("cannot parse command: %v", err))
	}
	if len(file.Stmts) != 1 {
		return nil, grepCommandError("shell_syntax", "grep accepts exactly one rg command")
	}
	stages := make([][]string, 0, 1)
	if err := appendLiteralGrepStages(file.Stmts[0], &stages); err != nil {
		return nil, err
	}
	return stages, nil
}

func appendLiteralGrepStages(statement *syntax.Stmt, stages *[][]string) error {
	if statement == nil {
		return grepCommandError("shell_syntax", "grep pipeline contains an empty command")
	}
	if statement.Background || statement.Coprocess || statement.Disown || statement.Negated || len(statement.Redirs) != 0 {
		return grepCommandError("shell_syntax", "backgrounding, redirection, and shell control syntax are unsupported")
	}
	if binary, ok := statement.Cmd.(*syntax.BinaryCmd); ok {
		if binary.Op != syntax.Pipe {
			return grepCommandError("shell_syntax", "only pipelines between literal rg commands are supported")
		}
		if err := appendLiteralGrepStages(binary.X, stages); err != nil {
			return err
		}
		return appendLiteralGrepStages(binary.Y, stages)
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return grepCommandError("shell_syntax", "grep accepts literal rg commands without assignments or substitutions")
	}
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		literal, ok := literalGrepWord(word)
		if !ok {
			return grepCommandError("shell_syntax", "variables, substitutions, and shell expansions are unsupported")
		}
		words = append(words, literal)
	}
	*stages = append(*stages, words)
	return nil
}

func literalGrepWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	var result strings.Builder
	var appendParts func([]syntax.WordPart) bool
	appendParts = func(parts []syntax.WordPart) bool {
		for _, part := range parts {
			switch value := part.(type) {
			case *syntax.Lit:
				result.WriteString(value.Value)
			case *syntax.SglQuoted:
				if value.Dollar {
					return false
				}
				result.WriteString(value.Value)
			case *syntax.DblQuoted:
				if value.Dollar || !appendParts(value.Parts) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	ok := appendParts(word.Parts)
	return result.String(), ok
}

func grepCommandError(code, message string) error {
	return fmt.Errorf("grep %s: %s", code, message)
}
