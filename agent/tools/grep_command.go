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

const maxGrepCommandBytes = 64 * 1024

// compileGrepCommand turns Denova's literal command language into an argv plan
// before any process starts. Positional arguments follow ripgrep's native
// PATTERN [PATH ...] contract; -e/--regexp makes every positional a path.
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
	words, err := parseLiteralGrepCommand(input)
	if err != nil {
		return compiledGrepCommand{}, err
	}
	if words[0] != "rg" {
		return compiledGrepCommand{}, grepCommandError("invalid_command", "command must begin with the literal executable rg", "命令必须以字面量 rg 开头")
	}
	command := compiledGrepCommand{mode: grepOutputContent}
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
		consumed, parseErr := command.consumeFlag(words, index)
		if parseErr != nil {
			return compiledGrepCommand{}, parseErr
		}
		command.args = append(command.args, words[index:index+consumed]...)
		index += consumed - 1
	}
	paths := positionals
	if !command.hasRegexp {
		if len(positionals) == 0 {
			return compiledGrepCommand{}, grepCommandError("invalid_pattern", "rg command must contain a pattern or -e/--regexp", "rg 命令必须包含 pattern 或 -e/--regexp")
		}
		// Canonicalizing the positional pattern through -e also preserves native
		// `rg -- -leading-dash path` behavior after Denova appends its invariants.
		command.args = append(command.args, "-e", positionals[0])
		command.hasRegexp = true
		paths = positionals[1:]
	}
	validated, warnings, err := workspace.validateGrepPaths(paths)
	if err != nil {
		return compiledGrepCommand{}, err
	}
	command.paths, command.warnings = validated, warnings
	return command, nil
}

func (command *compiledGrepCommand) consumeFlag(words []string, index int) (int, error) {
	token := words[index]
	if strings.HasPrefix(token, "--") {
		return command.consumeLongFlag(words, index)
	}
	return command.consumeShortFlags(words, index)
}

func (command *compiledGrepCommand) consumeLongFlag(words []string, index int) (int, error) {
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

func (command *compiledGrepCommand) consumeShortFlags(words []string, index int) (int, error) {
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

func (command *compiledGrepCommand) applyFlag(name, value string, spec grepFlagSpec) error {
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

func (command *compiledGrepCommand) selectMode(mode grepOutputMode, flag string) error {
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

func parseLiteralGrepCommand(input string) ([]string, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(input), "")
	if err != nil {
		return nil, grepCommandError("shell_syntax", fmt.Sprintf("cannot parse command: %v", err), "无法解析命令；请检查引号和转义")
	}
	if len(file.Stmts) != 1 {
		return nil, grepCommandError("shell_syntax", "grep accepts exactly one rg command", "grep 只接受一条 rg 命令")
	}
	statement := file.Stmts[0]
	if statement.Background || statement.Coprocess || statement.Disown || statement.Negated || len(statement.Redirs) != 0 {
		return nil, grepCommandError("shell_syntax", "backgrounding, redirection, and shell control syntax are unsupported", "不支持后台执行、重定向或 shell 控制语法")
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return nil, grepCommandError("shell_syntax", "grep accepts one literal rg invocation without assignments, pipelines, or substitutions", "grep 只接受不含赋值、管道或替换的一条字面 rg 调用")
	}
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		literal, ok := literalGrepWord(word)
		if !ok {
			return nil, grepCommandError("shell_syntax", "variables, substitutions, and shell expansions are unsupported", "不支持变量、命令替换或 shell 展开")
		}
		words = append(words, literal)
	}
	return words, nil
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
