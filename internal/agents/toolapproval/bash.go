package toolapproval

import (
	"strings"

	"denova/config"
	"mvdan.cc/sh/v3/syntax"
)

type commandClass uint8

const (
	commandUnknown commandClass = iota
	commandRead
	commandWrite
	commandNetworkRead
)

func classifyBash(command, workspace, cwd string, mode config.AgentApprovalMode) Decision {
	return classifyBashWithAllowedFiles(command, workspace, cwd, mode, nil)
}

func classifyBashWithAllowedFiles(command, workspace, cwd string, mode config.AgentApprovalMode, allowedFiles []string) Decision {
	boundary, ok := newPathBoundaryWithAllowedFiles(workspace, cwd, allowedFiles)
	if !ok {
		return prompt("invalid_workspace_boundary", RiskHigh,
			"无法验证命令工作目录，需要你的确认。 / The command working directory cannot be verified, so approval is required.")
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return prompt("bash_parse_failed", RiskHigh,
			"无法可靠解析 Bash 命令，需要你的确认。 / The Bash command could not be parsed reliably, so approval is required.")
	}
	calls, ok := staticBashCalls(file, boundary)
	if !ok || len(calls) == 0 {
		return prompt("bash_dynamic_syntax", RiskHigh,
			"命令包含无法静态验证的重定向、赋值、替换或复杂动态语法，需要你的确认。 / The command uses redirects, assignments, substitutions, or complex dynamic syntax that cannot be verified statically and requires approval.")
	}

	highest := commandRead
	for _, call := range calls {
		class := classifyLiteralCommand(call.words, call.boundary, mode)
		if class == commandUnknown {
			return prompt("bash_unlisted_command", RiskHigh,
				"命令不在当前模式的自动允许列表中，需要你的确认。 / The command is not in this mode's automatic allowlist and requires approval.")
		}
		if class > highest {
			highest = class
		}
	}
	switch highest {
	case commandRead:
		return allow("bash_safe_read", RiskLow,
			"命令仅包含白名单内的工作区读取操作。 / The command contains only allowlisted workspace read operations.")
	case commandWrite:
		return allow("bash_workspace_write", RiskMedium,
			"Write 模式允许该工作区开发或写入命令。 / Write mode allows this workspace development or write command.")
	case commandNetworkRead:
		return allow("bash_network_read", RiskMedium,
			"Write 模式允许该网络读取或下载命令。 / Write mode allows this network read or download command.")
	default:
		return prompt("bash_unclassified", RiskHigh,
			"命令无法安全分类，需要你的确认。 / The command cannot be safely classified and requires approval.")
	}
}

func literalBashCalls(file *syntax.File) ([][]string, bool) {
	valid := true
	calls := make([][]string, 0)
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil || !valid {
			return valid
		}
		switch value := node.(type) {
		case *syntax.Stmt:
			if value.Background || value.Coprocess || value.Disown || value.Negated || len(value.Redirs) != 0 {
				valid = false
				return false
			}
		case *syntax.CallExpr:
			if len(value.Assigns) != 0 || len(value.Args) == 0 {
				valid = false
				return false
			}
			words := make([]string, 0, len(value.Args))
			for _, word := range value.Args {
				literal, ok := literalBashWord(word)
				if !ok {
					valid = false
					return false
				}
				words = append(words, literal)
			}
			calls = append(calls, words)
			return false
		case *syntax.File, *syntax.BinaryCmd:
			return true
		default:
			// Word nodes are handled atomically above. Any other command form
			// (functions, loops, subshells, declarations, tests) is interactive.
			valid = false
			return false
		}
		return true
	})
	return calls, valid
}

func literalBashWord(word *syntax.Word) (string, bool) {
	return literalBashWordForPolicy(word, false)
}

func literalBashWordForPolicy(word *syntax.Word, allowUnquotedExpansion bool) (string, bool) {
	if word == nil {
		return "", false
	}
	var result strings.Builder
	var appendParts func([]syntax.WordPart) bool
	appendParts = func(parts []syntax.WordPart) bool {
		for _, part := range parts {
			switch value := part.(type) {
			case *syntax.Lit:
				// Unquoted glob and brace expansions can resolve to symlinks or
				// paths that were not present in the inspected argument.
				if !allowUnquotedExpansion && strings.ContainsAny(value.Value, "*?[]{}") {
					return false
				}
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

func classifyLiteralCommand(words []string, boundary pathBoundary, mode config.AgentApprovalMode) commandClass {
	if len(words) == 0 {
		return commandUnknown
	}
	name := strings.ToLower(words[0])
	args := words[1:]
	if safeVersionCommand(name, args) {
		return commandRead
	}
	switch name {
	case "cd":
		if _, ok := boundary.changeDirectory(args); ok {
			return commandRead
		}
	case "echo", "true", "false", ":":
		return commandRead
	case "printf":
		if safePrintfArguments(args) {
			return commandRead
		}
	case "awk":
		if safeAWKReadArguments(args, boundary) {
			return commandRead
		}
	case "pwd":
		if len(args) == 0 || len(args) == 1 && (args[0] == "-L" || args[0] == "-P") {
			return commandRead
		}
	case "ls", "tree", "stat", "file", "du", "df", "realpath", "readlink", "basename", "dirname":
		if safePathReadArguments(name, args, boundary) {
			return commandRead
		}
	case "cat", "head", "tail", "wc", "nl", "xxd", "cmp":
		if safeStreamReadArguments(name, args, boundary) {
			return commandRead
		}
	case "diff":
		if safeDiffArguments(args, boundary) {
			return commandRead
		}
	case "sort":
		if safeSortArguments(args, boundary) {
			return commandRead
		}
	case "uniq":
		if safeUniqArguments(args, boundary) {
			return commandRead
		}
	case "cut":
		if safeCutArguments(args, boundary) {
			return commandRead
		}
	case "tr":
		if safeTrArguments(args) {
			return commandRead
		}
	case "rg", "grep":
		if safeSearchArguments(name, args, boundary) {
			return commandRead
		}
	case "jq":
		if safeJQArguments(args, boundary) {
			return commandRead
		}
	case "find":
		if safeFindArguments(args, boundary) {
			return commandRead
		}
	case "git":
		if safeGitRead(args, boundary) {
			return commandRead
		}
	case "which", "whereis":
		if len(args) > 0 {
			return commandRead
		}
	case "type":
		if len(args) > 0 && !hasAnyFlag(args, "-f", "-p", "-P") {
			return commandRead
		}
	case "command":
		if len(args) >= 2 && (args[0] == "-v" || args[0] == "-V") {
			return commandRead
		}
	}
	if mode != config.AgentApprovalWrite {
		return commandUnknown
	}
	// Attached files extend only the read boundary. They are immutable inputs,
	// not extra workspace paths that Write mode may mutate automatically.
	writeBoundary := boundary
	writeBoundary.allowedFiles = nil
	switch name {
	case "mkdir", "touch":
		if safeSimpleWorkspaceWrite(name, args, writeBoundary) {
			return commandWrite
		}
	case "cp", "mv":
		if safeCopyMoveArguments(args, writeBoundary) {
			return commandWrite
		}
	case "rm", "rmdir":
		paths := nonFlagArguments(args)
		if len(paths) > 0 && !containsBroadWorkspaceTarget(writeBoundary, paths) && allPathsInside(writeBoundary, paths) {
			return commandWrite
		}
	case "tee":
		if len(args) > 0 && allPathsInside(writeBoundary, nonFlagArguments(args)) {
			return commandWrite
		}
	case "git":
		return classifyGitWrite(args, writeBoundary)
	case "go", "cargo", "make", "cmake", "ninja", "gradle", "gradlew", "mvn", "pytest", "ruff", "eslint", "prettier", "tsc", "vite":
		if safeDevelopmentCommand(name, args, writeBoundary) {
			return commandWrite
		}
	case "npm", "pnpm", "yarn", "bun", "npx", "bunx":
		if safePackageCommand(name, args, writeBoundary) {
			return commandWrite
		}
	case "python", "python3":
		if len(args) == 2 && args[0] == "-c" && strings.TrimSpace(args[1]) != "" {
			// Write already authorizes project code runners such as go run and
			// npm test. Inline Python has the same execution tier; this policy
			// intentionally does not pretend to understand Python semantics.
			return commandWrite
		}
	case "curl":
		if safeCurl(args, writeBoundary) {
			return commandNetworkRead
		}
	case "wget":
		if safeWget(args, writeBoundary) {
			return commandNetworkRead
		}
	}
	return commandUnknown
}
