package toolapproval

import "strings"

func safeVersionCommand(name string, args []string) bool {
	if len(args) != 1 || args[0] != "--version" && args[0] != "-V" && args[0] != "-v" {
		return false
	}
	switch name {
	case "bash", "zsh", "pwsh", "node", "npm", "pnpm", "yarn", "bun", "go", "python", "python3", "ruby", "java", "javac", "cargo", "rustc", "git", "rg", "grep", "jq", "make", "cmake", "ninja":
		return true
	default:
		return false
	}
}

func safeSearchArguments(name string, args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if name == "rg" && (hasShortOption(arg, 'L') || lower == "--follow") ||
			name == "grep" && (hasShortOption(arg, 'R') || lower == "--dereference-recursive") {
			return false
		}
		if name == "rg" && (lower == "--pre" || strings.HasPrefix(lower, "--pre=") || lower == "--config" || strings.HasPrefix(lower, "--config=")) {
			return false
		}
		if name == "rg" && optionPathInside(args, index, lower, "--ignore-file", boundary) == optionPathOutside {
			return false
		}
		if name == "grep" && (optionPathInside(args, index, lower, "-f", boundary) == optionPathOutside ||
			optionPathInside(args, index, lower, "--file", boundary) == optionPathOutside ||
			optionPathInside(args, index, lower, "--exclude-from", boundary) == optionPathOutside) {
			return false
		}
		if name == "grep" && strings.HasPrefix(arg, "-f") && arg != "-f" &&
			!boundary.containsLiteral(strings.TrimPrefix(arg, "-f")) {
			return false
		}
	}
	// Search patterns are not paths. Only reject explicit absolute/home paths
	// and relative traversal operands; ordinary literals remain valid patterns.
	for _, arg := range nonFlagArguments(args) {
		if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") || strings.HasPrefix(arg, "..") {
			if !boundary.containsLiteral(arg) {
				return false
			}
		}
	}
	return len(args) > 0
}

func safeJQArguments(args []string, boundary pathBoundary) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-f" || lower == "--from-file" || strings.HasPrefix(lower, "--from-file=") ||
			strings.HasPrefix(lower, "-f") ||
			lower == "-l" || strings.HasPrefix(lower, "-l") ||
			lower == "--library-path" || strings.HasPrefix(lower, "--library-path=") {
			return false
		}
	}
	operands := nonFlagArguments(args)
	if len(operands) == 0 {
		return false
	}
	// First operand is the filter; remaining operands, when present, are files.
	return allPathsInside(boundary, operands[1:])
}

func safeGitRead(args []string, boundary pathBoundary) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "--ext-diff" || lower == "--textconv" ||
			lower == "--open-files-in-pager" || strings.HasPrefix(lower, "--open-files-in-pager=") ||
			lower == "--output" || strings.HasPrefix(lower, "--output=") ||
			lower == "--paginate" || lower == "-p" || lower == "--no-index" ||
			lower == "--pathspec-from-file" || strings.HasPrefix(lower, "--pathspec-from-file=") {
			return false
		}
		if looksLikeExternalPath(arg) && !boundary.containsLiteral(arg) {
			return false
		}
	}
	switch args[0] {
	case "status", "diff", "log", "show", "grep", "ls-files", "rev-parse", "describe":
		return true
	case "remote":
		return len(args) == 1 || len(args) == 2 && args[1] == "-v"
	case "branch":
		return len(args) == 1 || len(args) == 2 && args[1] == "--show-current"
	default:
		return false
	}
}

func safeUniqArguments(args []string, boundary pathBoundary) bool {
	operands, ok := positionalArguments(args, map[string]bool{
		"-f": true, "--skip-fields": true,
		"-s": true, "--skip-chars": true,
		"-w": true, "--check-chars": true,
	})
	return ok && len(operands) <= 1 && allPathsInside(boundary, operands)
}

func safeCutArguments(args []string, boundary pathBoundary) bool {
	operands, ok := positionalArguments(args, map[string]bool{
		"-b": true, "--bytes": true,
		"-c": true, "--characters": true,
		"-d": true, "--delimiter": true,
		"-f": true, "--fields": true,
		"--output-delimiter": true,
	})
	return ok && allPathsInside(boundary, operands)
}

func safeTrArguments(args []string) bool {
	operands, ok := positionalArguments(args, nil)
	return ok && len(operands) >= 1 && len(operands) <= 2
}

func safeSimpleWorkspaceWrite(name string, args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if name == "touch" && (optionPathInside(args, index, lower, "-r", boundary) == optionPathOutside ||
			optionPathInside(args, index, lower, "--reference", boundary) == optionPathOutside) {
			return false
		}
		if name == "touch" && strings.HasPrefix(arg, "-r") && arg != "-r" &&
			!strings.HasPrefix(arg, "--") && !boundary.containsLiteral(strings.TrimPrefix(arg, "-r")) {
			return false
		}
	}
	paths := nonFlagArguments(args)
	return len(paths) > 0 && allPathsInside(boundary, paths)
}

func safeCopyMoveArguments(args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if optionPathInside(args, index, lower, "-t", boundary) == optionPathOutside ||
			optionPathInside(args, index, lower, "--target-directory", boundary) == optionPathOutside {
			return false
		}
		if strings.HasPrefix(arg, "-t") && arg != "-t" && !strings.HasPrefix(arg, "--") &&
			!boundary.containsLiteral(strings.TrimPrefix(arg, "-t")) {
			return false
		}
	}
	paths := nonFlagArguments(args)
	return len(paths) >= 2 && allPathsInside(boundary, paths)
}

func classifyGitWrite(args []string, boundary pathBoundary) commandClass {
	if len(args) == 0 || hasExternalLiteralArgument(args, boundary) {
		return commandUnknown
	}
	switch args[0] {
	case "add":
		if len(args) > 1 && !hasFlagPrefix(args[1:], "--pathspec-from-file") &&
			allPathsInside(boundary, nonFlagArguments(args[1:])) {
			return commandWrite
		}
	case "commit":
		if hasFlagPrefix(args[1:], "--pathspec-from-file") {
			return commandUnknown
		}
		return commandWrite
	case "fetch":
		return commandNetworkRead
	case "clone":
		if hasFlagPrefix(args[1:], "--separate-git-dir") ||
			hasFlagPrefix(args[1:], "--upload-pack") || hasFlagPrefix(args[1:], "-u") || hasShortFlagPrefix(args[1:], "-u") ||
			hasFlagPrefix(args[1:], "--config") || hasFlagPrefix(args[1:], "-c") {
			return commandUnknown
		}
		if len(args) >= 2 {
			destination := "."
			if len(args) >= 3 {
				destination = args[len(args)-1]
			}
			if boundary.containsLiteral(destination) {
				return commandNetworkRead
			}
		}
	case "pull":
		return commandNetworkRead
	}
	return commandUnknown
}

func safeDevelopmentCommand(name string, args []string, boundary pathBoundary) bool {
	if hasExternalLiteralArgument(args, boundary) ||
		containsActionArgument(args, "install", "uninstall", "publish", "unpublish", "deploy", "release", "upload") {
		return false
	}
	// These flags replace part of a trusted toolchain with an invocation chosen
	// by the command. Treat them as arbitrary execution instead of inheriting a
	// broad development-command policy or a saved command-family approval.
	switch name {
	case "go":
		if hasFlagPrefix(args, "-exec", "-toolexec", "-overlay") {
			return false
		}
	case "cargo":
		if hasFlagPrefix(args, "--config") {
			return false
		}
	}
	if name == "gradlew" {
		return true
	}
	if len(args) == 0 {
		return name == "make" || name == "ninja"
	}
	switch name {
	case "go":
		return oneOf(args[0], "test", "build", "run", "generate", "fmt", "vet") ||
			len(args) > 1 && args[0] == "mod" && oneOf(args[1], "download", "tidy", "verify")
	case "cargo":
		return oneOf(args[0], "test", "build", "check", "fmt", "clippy", "run", "fetch")
	default:
		return true
	}
}

func safePackageCommand(name string, args []string, boundary pathBoundary) bool {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	if containsArgument(args,
		"publish", "unpublish", "deploy", "release", "upload", "deprecate", "owner", "token", "login", "logout",
		"adduser", "profile", "access", "team", "dist-tag", "config", "set", "global", "link", "unlink",
	) || hasAnyFlag(args, "-g", "--global") || hasFlagPrefix(args,
		"--prefix", "--cwd", "--dir", "--global-dir", "--global-bin-dir",
		"--cache", "--store-dir", "--modules-dir", "--userconfig", "--globalconfig",
		"--location", "--global-folder",
	) {
		return false
	}
	runner := oneOf(name, "npx", "bunx") ||
		oneOf(strings.ToLower(args[0]), "exec", "dlx", "x")
	for _, arg := range args {
		if arg == "--" {
			break
		}
		option, _, _ := strings.Cut(strings.ToLower(arg), "=")
		if oneOf(option, "--script-shell", "--shell", "--node-options") ||
			runner && oneOf(option, "-c", "--call", "-p", "--package") {
			return false
		}
	}
	return !hasExternalLiteralArgument(args, boundary)
}

func containsBroadWorkspaceTarget(boundary pathBoundary, paths []string) bool {
	for _, value := range paths {
		value = strings.TrimSpace(value)
		switch value {
		case ".", "./", "":
			return true
		}
		if strings.ContainsAny(value, "*?[") || boundary.isWorkspaceRoot(value) {
			return true
		}
	}
	return false
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true
			}
		}
	}
	return false
}

type optionPathResult uint8

const (
	optionPathAbsent optionPathResult = iota
	optionPathAllowed
	optionPathOutside
)

func optionPathInside(args []string, index int, lower, option string, boundary pathBoundary) optionPathResult {
	if lower == option {
		if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
			return optionPathOutside
		}
		return optionPathAllowed
	}
	prefix := option + "="
	if strings.HasPrefix(lower, prefix) {
		if !boundary.containsLiteral(args[index][len(prefix):]) {
			return optionPathOutside
		}
		return optionPathAllowed
	}
	return optionPathAbsent
}

func positionalArguments(args []string, valueOptions map[string]bool) ([]string, bool) {
	result := make([]string, 0, len(args))
	flagsEnded := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !flagsEnded && arg == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "-") && arg != "-" {
			name := strings.ToLower(arg)
			if before, _, found := strings.Cut(name, "="); found {
				name = before
			}
			if valueOptions[name] && !strings.Contains(arg, "=") {
				if index+1 >= len(args) {
					return nil, false
				}
				index++
			}
			continue
		}
		result = append(result, arg)
	}
	return result, true
}

func looksLikeExternalPath(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		value == ".." || strings.HasPrefix(value, "../")
}

func hasExternalLiteralArgument(args []string, boundary pathBoundary) bool {
	for _, arg := range args {
		value := arg
		if _, optionValue, found := strings.Cut(arg, "="); found {
			value = optionValue
		}
		if looksLikeExternalPath(value) && !boundary.containsLiteral(value) {
			return true
		}
	}
	return false
}

func containsArgument(args []string, values ...string) bool {
	for _, arg := range args {
		for _, value := range values {
			if strings.EqualFold(strings.TrimSpace(arg), value) {
				return true
			}
		}
	}
	return false
}

func containsActionArgument(args []string, values ...string) bool {
	for _, arg := range args {
		normalized := strings.TrimLeft(strings.TrimSpace(arg), "-")
		normalized, _, _ = strings.Cut(normalized, "=")
		for _, value := range values {
			if strings.EqualFold(normalized, value) {
				return true
			}
		}
	}
	return false
}

func hasFlagPrefix(args []string, flags ...string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, flag := range flags {
			flag = strings.ToLower(flag)
			if lower == flag || strings.HasPrefix(lower, flag+"=") {
				return true
			}
		}
	}
	return false
}

func hasShortFlagPrefix(args []string, flag string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, flag) && arg != flag {
			return true
		}
	}
	return false
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}
