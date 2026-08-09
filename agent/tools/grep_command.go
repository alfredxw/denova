package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
		return compiledGrepCommand{}, grepCommandError("invalid_command", "command is required", "command 为必填项")
	}
	if len(input) > maxGrepCommandBytes {
		return compiledGrepCommand{}, grepCommandError(
			"invalid_command",
			fmt.Sprintf("command exceeds the %d-byte limit", maxGrepCommandBytes),
			fmt.Sprintf("command 超过 %d 字节上限", maxGrepCommandBytes),
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
			fmt.Sprintf("grep 管道超过 %d 段上限", maxGrepPipelineStages),
		)
	}
	command := compiledGrepCommand{stages: make([]compiledGrepStage, 0, len(pipeline))}
	for index, words := range pipeline {
		stage, warnings, err := workspace.compileGrepStage(words, index)
		if err != nil {
			return compiledGrepCommand{}, err
		}
		command.stages = append(command.stages, stage)
		command.warnings = append(command.warnings, warnings...)
	}
	return command, nil
}

// compileGrepStage preserves ripgrep's PATTERN [PATH ...] contract for the
// first stage. Later stages receive only the preceding stdout and therefore
// reject workspace paths instead of silently ignoring the pipeline input.
func (workspace *LocalWorkspace) compileGrepStage(words []string, stageIndex int) (compiledGrepStage, []string, error) {
	if len(words) == 0 || words[0] != "rg" {
		return compiledGrepStage{}, nil, grepCommandError(
			"invalid_command",
			fmt.Sprintf("pipeline stage %d must begin with the literal executable rg", stageIndex+1),
			fmt.Sprintf("管道第 %d 段命令必须以字面量 rg 开头", stageIndex+1),
		)
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
			return compiledGrepStage{}, nil, parseErr
		}
		stage.args = append(stage.args, words[index:index+consumed]...)
		index += consumed - 1
	}
	paths := positionals
	if !stage.hasRegexp {
		if len(positionals) == 0 {
			return compiledGrepStage{}, nil, grepCommandError("invalid_pattern", "rg command must contain a pattern or -e/--regexp", "rg 命令必须包含 pattern 或 -e/--regexp")
		}
		// Canonicalizing the positional pattern through -e also preserves native
		// `rg -- -leading-dash path` behavior after Denova appends its invariants.
		stage.args = append(stage.args, "-e", positionals[0])
		stage.hasRegexp = true
		paths = positionals[1:]
	}
	if stageIndex > 0 {
		if len(paths) > 1 || (len(paths) == 1 && paths[0] != "-") {
			return compiledGrepStage{}, nil, grepCommandError(
				"pipeline_path",
				fmt.Sprintf("pipeline stage %d must read only from the preceding rg stage", stageIndex+1),
				fmt.Sprintf("管道第 %d 段只能读取上一段 rg 的输出", stageIndex+1),
			)
		}
		stage.paths = []string{"-"}
		return stage, nil, nil
	}
	validated, warnings, err := workspace.validateGrepPaths(paths)
	if err != nil {
		return compiledGrepStage{}, nil, err
	}
	stage.paths = validated
	return stage, warnings, nil
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
		return 0, grepCommandError("unsafe_flag", fmt.Sprintf("--%s is unavailable: %s", name, reason), fmt.Sprintf("--%s 不可用：该参数超出受控搜索范围", name))
	}
	spec, ok := grepLongFlagSpecs[name]
	if !ok {
		return 0, grepCommandError("unsupported_flag", fmt.Sprintf("unsupported ripgrep flag --%s", name), fmt.Sprintf("不支持 ripgrep 参数 --%s", name))
	}
	consumed := 1
	if spec.value {
		if !attached {
			if index+1 >= len(words) || words[index+1] == "--" {
				return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("--%s requires a value", name), fmt.Sprintf("--%s 需要参数值", name))
			}
			value = words[index+1]
			consumed = 2
		}
	} else if attached {
		return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("--%s does not accept a value", name), fmt.Sprintf("--%s 不接受参数值", name))
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
		return 0, grepCommandError("invalid_pattern", "use -e '-' to search for a literal dash", "如需搜索字面量短横线，请使用 -e '-'")
	}
	consumed := 1
	for offset := 0; offset < len(body); offset++ {
		name := body[offset]
		if reason, unsafe := grepUnsafeShortFlags[name]; unsafe {
			return 0, grepCommandError("unsafe_flag", fmt.Sprintf("-%c is unavailable: %s", name, reason), fmt.Sprintf("-%c 不可用：该参数超出受控搜索范围", name))
		}
		spec, ok := grepShortFlagSpecs[name]
		if !ok {
			return 0, grepCommandError("unsupported_flag", fmt.Sprintf("unsupported ripgrep flag -%c", name), fmt.Sprintf("不支持 ripgrep 参数 -%c", name))
		}
		value := ""
		if spec.value {
			if offset+1 < len(body) {
				value = body[offset+1:]
			} else {
				if index+1 >= len(words) || words[index+1] == "--" {
					return 0, grepCommandError("invalid_flag_value", fmt.Sprintf("-%c requires a value", name), fmt.Sprintf("-%c 需要参数值", name))
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
			return grepCommandError("invalid_flag_value", fmt.Sprintf("%s has invalid value %q: %v", name, value, err), fmt.Sprintf("%s 的参数值 %q 无效", name, value))
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
		return grepCommandError("conflicting_flags", fmt.Sprintf("output modes %s and %s cannot be combined", command.modeFlag, flag), fmt.Sprintf("输出模式 %s 与 %s 不能同时使用", command.modeFlag, flag))
	}
	command.mode, command.modeFlag = mode, flag
	return nil
}

func (workspace *LocalWorkspace) validateGrepPaths(inputs []string) ([]string, []string, error) {
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	paths := make([]string, 0, len(inputs))
	warnings := make([]string, 0)
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		plain := filepath.ToSlash(strings.TrimSpace(input))
		if plain == "" || hasResourceScheme(plain) || hasParentComponent(plain) || isAbsoluteGrepPath(plain) {
			return nil, nil, grepCommandError("unsafe_path", fmt.Sprintf("grep path must be a literal workspace-relative path: %q", input), fmt.Sprintf("grep 路径必须是工作区相对的字面路径：%q", input))
		}
		relative, info, err := workspace.stat(plain, true)
		if err != nil {
			if hasGlobMeta(plain) {
				return nil, nil, grepCommandError(
					"path_glob",
					fmt.Sprintf("path glob %q is not expanded; use -g/--glob to filter files", input),
					fmt.Sprintf("路径 glob %q 不会被展开；请使用 -g/--glob 过滤文件", input),
				)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, grepCommandError(
					"unsafe_path",
					fmt.Sprintf("grep path %q cannot be safely inspected: %v", input, err),
					fmt.Sprintf("无法在工作区边界内安全检查 grep 路径 %q", input),
				)
			}
			if len(inputs) == 1 {
				return nil, nil, grepCommandError(
					"path_not_found",
					fmt.Sprintf("grep path %q does not exist", input),
					fmt.Sprintf("grep 路径 %q 不存在", input),
				)
			}
			warnings = append(warnings, fmt.Sprintf("Skipped missing path %q. / 已跳过不存在的路径 %q。", input, input))
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, nil, grepCommandError(
				"unsupported_path",
				fmt.Sprintf("grep path %q must be a regular file or directory", input),
				fmt.Sprintf("grep 路径 %q 必须是普通文件或目录", input),
			)
		}
		if _, duplicate := seen[relative]; duplicate {
			continue
		}
		seen[relative] = struct{}{}
		paths = append(paths, relative)
	}
	if len(paths) == 0 {
		return nil, nil, grepCommandError("no_searchable_path", "none of the requested grep paths exists", "请求的 grep 路径均不存在")
	}
	sort.Strings(paths)
	return paths, warnings, nil
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
		return nil, grepCommandError("shell_syntax", fmt.Sprintf("cannot parse command: %v", err), "无法解析命令；请检查引号和转义")
	}
	if len(file.Stmts) != 1 {
		return nil, grepCommandError("shell_syntax", "grep accepts exactly one rg command", "grep 只接受一条 rg 命令")
	}
	stages := make([][]string, 0, 1)
	if err := appendLiteralGrepStages(file.Stmts[0], &stages); err != nil {
		return nil, err
	}
	return stages, nil
}

func appendLiteralGrepStages(statement *syntax.Stmt, stages *[][]string) error {
	if statement == nil {
		return grepCommandError("shell_syntax", "grep pipeline contains an empty command", "grep 管道包含空命令")
	}
	if statement.Background || statement.Coprocess || statement.Disown || statement.Negated || len(statement.Redirs) != 0 {
		return grepCommandError("shell_syntax", "backgrounding, redirection, and shell control syntax are unsupported", "不支持后台执行、重定向或 shell 控制语法")
	}
	if binary, ok := statement.Cmd.(*syntax.BinaryCmd); ok {
		if binary.Op != syntax.Pipe {
			return grepCommandError("shell_syntax", "only pipelines between literal rg commands are supported", "仅支持字面量 rg 命令之间的管道")
		}
		if err := appendLiteralGrepStages(binary.X, stages); err != nil {
			return err
		}
		return appendLiteralGrepStages(binary.Y, stages)
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return grepCommandError("shell_syntax", "grep accepts literal rg commands without assignments or substitutions", "grep 只接受不含赋值或替换的字面量 rg 命令")
	}
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		literal, ok := literalGrepWord(word)
		if !ok {
			return grepCommandError("shell_syntax", "variables, substitutions, and shell expansions are unsupported", "不支持变量、命令替换或 shell 展开")
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

func grepCommandError(code, english, chinese string) error {
	return fmt.Errorf("grep %s: %s / %s", code, english, chinese)
}
