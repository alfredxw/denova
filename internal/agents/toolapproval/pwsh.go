package toolapproval

import (
	"regexp"
	"strings"
	"unicode"

	"denova/config"
)

var powerShellProviderPath = regexp.MustCompile(`(?i)^[a-z][a-z0-9.+-]*:`)

func classifyPowerShell(command, workspace, cwd string, mode config.AgentApprovalMode) Decision {
	return classifyPowerShellWithAllowedFiles(command, workspace, cwd, mode, nil)
}

func classifyPowerShellWithAllowedFiles(command, workspace, cwd string, mode config.AgentApprovalMode, allowedFiles []string) Decision {
	boundary, ok := newPathBoundaryWithAllowedFiles(workspace, cwd, allowedFiles)
	if !ok {
		return prompt("invalid_workspace_boundary", RiskHigh,
			"无法验证命令工作目录，需要你的确认。 / The command working directory cannot be verified, so approval is required.")
	}
	if strings.ContainsAny(command, "$`{}()><") || strings.Contains(command, "&&") || strings.Contains(command, "||") {
		return prompt("pwsh_dynamic_syntax", RiskHigh,
			"PowerShell 命令包含动态表达式、重定向或复杂语法，需要你的确认。 / The PowerShell command uses dynamic expressions, redirects, or complex syntax and requires approval.")
	}
	segments := strings.Split(command, "|")
	highest := commandRead
	for _, segment := range segments {
		words, ok := splitSimplePowerShellWords(segment)
		if !ok || len(words) == 0 {
			return prompt("pwsh_parse_failed", RiskHigh,
				"无法可靠解析 PowerShell 命令，需要你的确认。 / The PowerShell command could not be parsed reliably, so approval is required.")
		}
		class := classifyPowerShellCall(words, boundary, mode)
		if class == commandUnknown {
			return prompt("pwsh_unlisted_command", RiskHigh,
				"命令不在当前模式的自动允许列表中，需要你的确认。 / The command is not in this mode's automatic allowlist and requires approval.")
		}
		if class > highest {
			highest = class
		}
	}
	if highest == commandRead {
		return allow("pwsh_safe_read", RiskLow,
			"命令仅包含白名单内的工作区读取操作。 / The command contains only allowlisted workspace read operations.")
	}
	if highest == commandNetworkRead {
		return allow("pwsh_network_read", RiskMedium,
			"Write 模式允许该网络读取或下载命令。 / Write mode allows this network read or download command.")
	}
	return allow("pwsh_workspace_write", RiskMedium,
		"Write 模式允许该工作区开发或写入命令。 / Write mode allows this workspace development or write command.")
}

func splitSimplePowerShellWords(input string) ([]string, bool) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, char := range strings.TrimSpace(input) {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '`' {
			return nil, false
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ';' || char == '&' {
			return nil, false
		}
		if unicode.IsSpace(char) {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, false
	}
	flush()
	return words, true
}

func classifyPowerShellCall(words []string, boundary pathBoundary, mode config.AgentApprovalMode) commandClass {
	name := strings.ToLower(words[0])
	args := words[1:]
	switch name {
	case "get-location", "pwd", "get-childitem", "gci", "ls", "dir":
		if powerShellPathsInside(args, boundary) {
			return commandRead
		}
	case "get-content", "gc", "cat", "type", "measure-object", "select-string", "compare-object", "get-item", "get-command":
		if powerShellPathsInside(args, boundary) {
			return commandRead
		}
	case "git":
		if safeGitRead(args, boundary) {
			return commandRead
		}
	}
	if mode != config.AgentApprovalWrite {
		return commandUnknown
	}
	switch name {
	case "new-item", "set-content", "add-content", "copy-item", "move-item", "remove-item":
		paths := powerShellPathArguments(args)
		if name == "remove-item" && containsBroadWorkspaceTarget(boundary, paths) {
			return commandUnknown
		}
		if allPathsInside(boundary, paths) {
			return commandWrite
		}
	case "git":
		return classifyGitWrite(args, boundary)
	case "npm", "pnpm", "yarn", "bun", "npx", "bunx":
		if safePackageCommand(name, args, boundary) {
			return commandWrite
		}
	case "go", "cargo", "dotnet", "msbuild":
		if safeDevelopmentCommand(name, args, boundary) || (name == "dotnet" || name == "msbuild") &&
			!hasExternalLiteralArgument(args, boundary) && !containsArgument(args, "publish", "deploy") {
			return commandWrite
		}
	case "invoke-webrequest", "iwr":
		if powerShellNetworkRead(args, boundary) {
			return commandNetworkRead
		}
	}
	return commandUnknown
}

func powerShellPathsInside(args []string, boundary pathBoundary) bool {
	for _, value := range powerShellPathArguments(args) {
		value = strings.TrimSpace(value)
		if powerShellProviderPath.MatchString(value) &&
			!(len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')) {
			return false
		}
	}
	return allPathsInside(boundary, powerShellPathArguments(args))
}

func powerShellPathArguments(args []string) []string {
	paths := make([]string, 0)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			if powerShellValueParameter(arg) && index+1 < len(args) {
				index++
				paths = append(paths, args[index])
			}
			continue
		}
		paths = append(paths, arg)
	}
	return paths
}

func powerShellValueParameter(value string) bool {
	switch strings.ToLower(value) {
	case "-path", "-literalpath", "-destination", "-file", "-outfile":
		return true
	default:
		return false
	}
}

func powerShellNetworkRead(args []string, boundary pathBoundary) bool {
	uri := ""
	for index, arg := range args {
		switch strings.ToLower(arg) {
		case "-uri":
			if index+1 >= len(args) {
				return false
			}
			uri = args[index+1]
		case "-method":
			if index+1 >= len(args) || !strings.EqualFold(args[index+1], "get") {
				return false
			}
		case "-body", "-form", "-infile":
			return false
		case "-credential", "-usedefaultcredentials", "-authentication", "-token":
			return false
		case "-outfile":
			if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
				return false
			}
		}
	}
	if uri == "" {
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				uri = arg
				break
			}
		}
	}
	lowerURI := strings.ToLower(strings.TrimSpace(uri))
	return strings.HasPrefix(lowerURI, "https://") || strings.HasPrefix(lowerURI, "http://")
}
