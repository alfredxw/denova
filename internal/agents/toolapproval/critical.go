package toolapproval

import (
	"os"
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type criticalRule struct {
	id      string
	pattern *regexp.Regexp
	reason  string
}

var criticalCommandRules = []criticalRule{
	{
		id:      "critical_recursive_root_delete",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?rm\s+(?:-[a-z]*r[a-z]*f[a-z]*|-[a-z]*f[a-z]*r[a-z]*|--recursive(?:\s+--force)?|--force\s+--recursive)\s+(?:--\s+)?(?:/|~|\$HOME|\$\{HOME\})(?:\s|$)`),
		reason:  "禁止递归强制删除根目录或用户目录。 / Recursive forced deletion of a root or home directory is blocked.",
	},
	{
		id:      "critical_fork_bomb",
		pattern: regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
		reason:  "禁止执行 fork bomb。 / Fork bombs are blocked.",
	},
	{
		id:      "critical_disk_format",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:mkfs(?:\.[a-z0-9]+)?|wipefs|fdisk|parted)\b`),
		reason:  "禁止格式化或重写磁盘分区。 / Disk formatting and partition rewriting are blocked.",
	},
	{
		id:      "critical_raw_device_write",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?dd\b[^\n;&|]*\bof\s*=\s*/dev/(?:sd|hd|vd|nvme|disk|rdisk)`),
		reason:  "禁止向原始块设备写入数据。 / Writes to raw block devices are blocked.",
	},
	{
		id:      "critical_raw_device_redirect",
		pattern: regexp.MustCompile(`(?i)(?:>|>>)\s*/dev/(?:sd|hd|vd|nvme|disk|rdisk)[a-z0-9]*\b`),
		reason:  "禁止通过重定向写入原始块设备。 / Redirected writes to raw block devices are blocked.",
	},
	{
		id:      "critical_device_destroy",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:shred\b[^\n;&|]*/dev/|cryptsetup\b)`),
		reason:  "禁止擦除块设备或重写磁盘加密元数据。 / Block-device shredding and disk-encryption metadata changes are blocked.",
	},
	{
		id:      "critical_auth_files",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:rm|mv|cp|install|chmod|chown|truncate|tee|sed)\b[^\n;&|]*(?:/etc/(?:passwd|shadow|sudoers)|/etc/sudoers\.d/)`),
		reason:  "禁止修改系统认证与 sudo 配置。 / Changes to system authentication and sudo configuration are blocked.",
	},
	{
		id:      "critical_auth_file_redirect",
		pattern: regexp.MustCompile(`(?i)(?:>|>>)\s*(?:/etc/(?:passwd|shadow|sudoers)|/etc/sudoers\.d/)`),
		reason:  "禁止通过重定向修改系统认证与 sudo 配置。 / Redirected writes to system authentication and sudo configuration are blocked.",
	},
	{
		id:      "critical_power_control",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:shutdown|reboot|poweroff|halt)\b`),
		reason:  "禁止关闭或重启宿主系统。 / Host shutdown and reboot commands are blocked.",
	},
	{
		id:      "critical_service_power_control",
		pattern: regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:(?:systemctl|loginctl)\b[^\r\n;&|]*(?:poweroff|reboot|halt)\b|(?:telinit|init)\s+[06]\b|launchctl\s+reboot\b)`),
		reason:  "禁止通过系统服务管理器关闭或重启宿主系统。 / Host shutdown and reboot through system service managers are blocked.",
	},
	{
		id:      "critical_process_one",
		pattern: regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:kill\s+(?:-9|-kill|-[a-z]*k[a-z]*)\s+1\b|init\s+0\b)`),
		reason:  "禁止终止宿主 init 进程或切换到关机运行级别。 / Terminating host init or switching to the shutdown runlevel is blocked.",
	},
	{
		id:      "critical_download_pipe_shell",
		pattern: regexp.MustCompile(`(?i)(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba|z|k|fi)?sh\b|(?:invoke-webrequest|iwr|curl)\b[^\r\n|]*\|\s*(?:invoke-expression|iex)\b`),
		reason:  "禁止将网络下载内容直接管道执行。 / Piping downloaded content directly into a shell is blocked.",
	},
	{
		id:      "critical_download_shell_substitution",
		pattern: regexp.MustCompile("(?i)(?:\\beval\\s+[\"'`]?\\$\\(\\s*(?:curl|wget)\\b|\\beval\\s+`\\s*(?:curl|wget)\\b|(?:^|[\\s;&|(])(?:bash|sh|zsh|source|\\.)\\s+<\\(\\s*(?:curl|wget)\\b)"),
		reason:  "禁止通过命令替换或进程替换执行网络下载内容。 / Executing downloaded content through command or process substitution is blocked.",
	},
	{
		id:      "critical_download_shell_command",
		pattern: regexp.MustCompile(`(?i)\b(?:ba|z|k|fi)?sh\b[^\r\n;&|]*-(?:c|lc)\s+["']?\$\(\s*(?:curl|wget)\b`),
		reason:  "禁止通过 Shell 命令字符串执行网络下载内容。 / Executing downloaded content through a shell command string is blocked.",
	},
	{
		id:      "critical_powershell_download_execute",
		pattern: regexp.MustCompile(`(?i)\b(?:iex|invoke-expression)\b[^\r\n;|]*(?:iwr|invoke-webrequest|curl|wget)\b`),
		reason:  "禁止通过 PowerShell 直接执行网络下载内容。 / Executing downloaded content through PowerShell is blocked.",
	},
	{
		id:      "critical_quoted_home_delete",
		pattern: regexp.MustCompile(`(?i)\brm\b[^\r\n;&|]*-[a-z]*r[a-z]*[^\r\n;&|]*(?:"\$(?:HOME|\{HOME\})(?:/\*)?"|'\$(?:HOME|\{HOME\})(?:/\*)?')`),
		reason:  "禁止递归删除用户目录。 / Recursive deletion of the home directory is blocked.",
	},
	{
		id:      "critical_find_root_delete",
		pattern: regexp.MustCompile(`(?i)\bfind\s+(?:/|~|\$HOME|\$\{HOME\})(?:\s|$)[^\r\n|]*\|\s*xargs\b[^\r\n|]*(?:rm|unlink|rmdir|shred)\b`),
		reason:  "禁止通过 find/xargs 递归删除根目录或用户目录。 / Recursive root or home deletion through find/xargs is blocked.",
	},
	{
		id:      "critical_find_symbolic_home_delete",
		pattern: regexp.MustCompile(`(?i)\bfind\s+(?:"\$(?:HOME|\{HOME\})"|'?\$(?:HOME|\{HOME\})'?|~|/)(?:\s|$)[^\r\n|]*(?:-delete\b|-exec(?:dir)?\s+(?:sudo\s+)?(?:rm|unlink|rmdir|shred)\b)`),
		reason:  "禁止通过 find 递归删除根目录或用户目录。 / Recursive root or home deletion through find is blocked.",
	},
	{
		id:      "critical_reverse_shell",
		pattern: regexp.MustCompile(`(?i)(?:\b(?:nc|ncat|netcat)\b[^\n;&|]*(?:\s-e\s|\s--exec\s)|/dev/(?:tcp|udp)/[^/\s]+/\d+)`),
		reason:  "禁止常见反向 Shell 命令。 / Common reverse-shell commands are blocked.",
	},
	{
		id:      "critical_powershell_disk",
		pattern: regexp.MustCompile(`(?i)\b(?:format-volume|clear-disk|initialize-disk|remove-partition)\b`),
		reason:  "禁止 PowerShell 磁盘破坏命令。 / Destructive PowerShell disk commands are blocked.",
	},
	{
		id:      "critical_powershell_power",
		pattern: regexp.MustCompile(`(?i)\b(?:stop-computer|restart-computer)\b`),
		reason:  "禁止 PowerShell 关闭或重启宿主系统。 / PowerShell host shutdown and restart commands are blocked.",
	},
	{
		id:      "critical_powershell_root_delete",
		pattern: regexp.MustCompile(`(?i)\bremove-item\b[^\r\n;|]*(?:-[a-z]*recurse\b[^\r\n;|]*-[a-z]*force|-[a-z]*force\b[^\r\n;|]*-[a-z]*recurse)[^\r\n;|]*(?:[a-z]:\\|[a-z]:/|~|\$home)(?:\s|$)`),
		reason:  "禁止递归强制删除磁盘根目录或用户目录。 / Recursive forced deletion of a drive root or home directory is blocked.",
	},
	{
		id:      "critical_windows_disk",
		pattern: regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:format(?:\.com)?\s+[a-z]:|diskpart\b|cipher(?:\.exe)?\s+/w:[a-z]:)`),
		reason:  "禁止格式化、擦除或重写 Windows 磁盘。 / Windows disk formatting, wiping, and partition changes are blocked.",
	},
	{
		id:      "critical_windows_root_delete",
		pattern: regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:rd|rmdir|del|erase)(?:\.exe)?\b[^\r\n;&|]*/s\b[^\r\n;&|]*[a-z]:[\\/](?:\*|\s|$)`),
		reason:  "禁止递归删除 Windows 磁盘根目录。 / Recursive deletion of a Windows drive root is blocked.",
	},
	{
		id:      "critical_windows_power",
		pattern: regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:shutdown(?:\.exe)?\b[^\r\n;&|]*(?:/s|/r|/p|/h)\b|wmic\b[^\r\n;&|]*\bcall\s+(?:shutdown|reboot)\b)`),
		reason:  "禁止通过 Windows 系统命令关闭或重启宿主系统。 / Windows host shutdown and reboot commands are blocked.",
	},
	{
		id:      "critical_powershell_encoded_command",
		pattern: regexp.MustCompile(`(?i)\b(?:pwsh|powershell)(?:\.exe)?\b[^\r\n;|]*\s-(?:e|ec|enc|enco|encod|encode|encoded|encodedc|encodedcommand)\b`),
		reason:  "禁止通过编码后的 PowerShell 命令绕过高危规则。 / Encoded PowerShell commands that bypass critical inspection are blocked.",
	},
}

var (
	powerShellDriveRootPattern = regexp.MustCompile(`(?i)^[a-z]:\\?(?:\*)?$`)
	rawDevicePathPattern       = regexp.MustCompile(`^/dev/(?:(?:sd|hd|vd|xvd)[a-z][0-9]*|nvme[0-9]+n[0-9]+(?:p[0-9]+)?|mmcblk[0-9]+(?:p[0-9]+)?|md[0-9]+|loop[0-9]+|r?disk[0-9]+(?:s[0-9]+)?|mapper/[^/]+|disk/by-(?:id|path)/.+|k?mem)$`)
)

func matchCriticalCommand(command, toolName, goos string) *Decision {
	normalized := strings.TrimSpace(command)
	for _, rule := range criticalCommandRules {
		if rule.pattern.MatchString(normalized) {
			result := deny(rule.id, RiskCritical, rule.reason)
			return &result
		}
	}
	if usesPowerShell(toolName, goos) {
		if decision := criticalPowerShellCommand(normalized); decision != nil {
			return decision
		}
	} else if decision := criticalBashCommand(normalized); decision != nil {
		return decision
	}
	return nil
}

func criticalBashCommand(command string) *Decision {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return nil
	}
	var blocked *Decision
	syntax.Walk(file, func(node syntax.Node) bool {
		if blocked != nil || node == nil {
			return blocked == nil
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		words := make([]string, 0, len(call.Args))
		for _, word := range call.Args {
			value, literal := literalBashWordForPolicy(word, true)
			if !literal {
				return false
			}
			words = append(words, value)
		}
		words = unwrapCommandPrefix(words)
		if len(words) == 0 {
			return false
		}
		name := commandBase(words[0])
		if name == "busybox" && len(words) > 1 {
			words = words[1:]
			name = commandBase(words[0])
		}
		if result := criticalInvocation(name, words[1:]); result != nil {
			blocked = result
			return false
		}
		if name == "rm" && criticalRecursiveDelete(words[1:]) {
			result := deny("critical_recursive_root_delete", RiskCritical,
				"禁止递归强制删除根目录或用户目录。 / Recursive forced deletion of a root or home directory is blocked.")
			blocked = &result
			return false
		}
		if oneOf(name, "chmod", "chown") && criticalRecursiveRootMetadata(words[1:]) {
			result := deny("critical_recursive_root_permissions", RiskCritical,
				"禁止递归修改根目录权限或所有者。 / Recursive permission or ownership changes on a root directory are blocked.")
			blocked = &result
			return false
		}
		if oneOf(name, "cp", "mv", "install", "tee", "truncate", "shred") && criticalDeviceWrite(words[1:]) {
			result := deny("critical_raw_device_write", RiskCritical,
				"禁止通过文件工具写入原始块设备。 / Writes to raw block devices through file utilities are blocked.")
			blocked = &result
			return false
		}
		if name == "find" && criticalFindDelete(words[1:]) {
			result := deny("critical_recursive_root_delete", RiskCritical,
				"禁止递归删除根目录、用户目录或整个工作目录。 / Recursive deletion of a root, home, or entire working directory is blocked.")
			blocked = &result
			return false
		}
		if name == "eval" && len(words) > 1 {
			if nested := matchCriticalCommand(strings.Join(words[1:], " "), "bash", ""); nested != nil {
				blocked = nested
				return false
			}
		}
		if name == "xargs" {
			if nestedCommand := xargsCommand(words[1:]); len(nestedCommand) > 0 {
				if nested := matchCriticalCommand(strings.Join(nestedCommand, " "), "bash", ""); nested != nil {
					blocked = nested
					return false
				}
			}
		}
		if oneOf(name, "sh", "bash", "zsh", "ksh", "fish", "pwsh", "powershell") {
			for index, arg := range words[1:] {
				if shellCommandStringFlag(name, arg) && index+2 < len(words) {
					if nested := matchCriticalCommand(words[index+2], name, ""); nested != nil {
						blocked = nested
					}
					break
				}
			}
		}
		return false
	})
	return blocked
}

func shellCommandStringFlag(shell, argument string) bool {
	if oneOf(shell, "pwsh", "powershell") {
		argument = strings.ToLower(argument)
		return argument == "-c" || strings.HasPrefix("-command", argument)
	}
	return len(argument) > 1 && argument[0] == '-' && argument[1] != '-' &&
		strings.ContainsRune(argument[1:], 'c')
}

func unwrapCommandPrefix(words []string) []string {
	for len(words) > 0 {
		switch commandBase(words[0]) {
		case "sudo", "command", "nohup", "exec":
			words = words[1:]
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				words = words[1:]
			}
		case "env":
			words = words[1:]
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || strings.Contains(words[0], "=")) {
				words = words[1:]
			}
		default:
			return words
		}
	}
	return words
}

func commandBase(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	return strings.ToLower(path.Base(value))
}

func criticalRecursiveDelete(args []string) bool {
	recursive := false
	operands := make([]string, 0, len(args))
	flagsEnded := false
	for _, arg := range args {
		if !flagsEnded && arg == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "--") {
			switch arg {
			case "--recursive":
				recursive = true
			}
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "-") && arg != "-" {
			flags := strings.TrimPrefix(arg, "-")
			recursive = recursive || strings.ContainsAny(flags, "rR")
			continue
		}
		operands = append(operands, arg)
	}
	if !recursive {
		return false
	}
	for _, operand := range operands {
		if criticalDeleteTarget(operand) {
			return true
		}
	}
	return false
}

func criticalRecursiveRootMetadata(args []string) bool {
	recursive := false
	operands := make([]string, 0, len(args))
	flagsEnded := false
	for _, arg := range args {
		if !flagsEnded && arg == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && strings.HasPrefix(arg, "-") && arg != "-" {
			recursive = recursive || arg == "--recursive" || strings.Contains(arg, "R")
			continue
		}
		operands = append(operands, arg)
	}
	if !recursive || len(operands) < 2 {
		return false
	}
	for _, operand := range operands[1:] {
		if criticalDeleteTarget(operand) {
			return true
		}
	}
	return false
}

func criticalFindDelete(args []string) bool {
	rootTarget := false
	deleteAction := false
	destructiveExec := false
	for index, arg := range args {
		if criticalDeleteTarget(arg) {
			rootTarget = true
		}
		if arg == "-delete" {
			deleteAction = true
		}
		if oneOf(arg, "-exec", "-execdir") && index+1 < len(args) {
			end := index + 1
			for end < len(args) && args[end] != ";" && args[end] != "+" {
				end++
			}
			nestedWords := unwrapCommandPrefix(args[index+1 : end])
			if len(nestedWords) > 0 && oneOf(commandBase(nestedWords[0]), "rm", "unlink", "rmdir", "shred") {
				destructiveExec = true
			}
			if nested := matchCriticalCommand(strings.Join(nestedWords, " "), "bash", ""); nested != nil {
				return true
			}
		}
	}
	return rootTarget && (deleteAction || destructiveExec)
}

func criticalDeviceWrite(args []string) bool {
	for _, argument := range nonFlagArguments(args) {
		if criticalRawDeviceTarget(argument) {
			return true
		}
	}
	return false
}

func xargsCommand(args []string) []string {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			return args[index+1:]
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return args[index:]
	}
	return nil
}

func criticalPowerShellCommand(command string) *Decision {
	for _, segment := range strings.FieldsFunc(command, func(char rune) bool { return char == '|' || char == ';' }) {
		words, ok := splitSimplePowerShellWords(segment)
		if !ok || len(words) == 0 ||
			!oneOf(commandBase(words[0]), "remove-item", "rm", "ri", "del", "erase", "rmdir", "rd") {
			continue
		}
		recursive := false
		for _, word := range words[1:] {
			switch strings.ToLower(word) {
			case "-recurse", "-r":
				recursive = true
			}
		}
		if !recursive {
			continue
		}
		for _, word := range words[1:] {
			value := strings.TrimSpace(strings.ReplaceAll(word, "/", "\\"))
			if value == "~" || value == "~\\*" ||
				strings.EqualFold(value, "$HOME") || strings.EqualFold(value, "$HOME\\*") ||
				strings.EqualFold(value, "$env:USERPROFILE") || strings.EqualFold(value, "$env:USERPROFILE\\*") ||
				powerShellDriveRootPattern.MatchString(value) || criticalDeleteTarget(value) {
				result := deny("critical_powershell_root_delete", RiskCritical,
					"禁止递归删除磁盘根目录或用户目录。 / Recursive deletion of a drive root or home directory is blocked.")
				return &result
			}
		}
	}
	return nil
}

func criticalDeleteTarget(value string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	normalized = strings.Trim(normalized, `"'`)
	if normalized == "" {
		return false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "/" || strings.HasPrefix(normalized, "/*") {
		return true
	}
	for _, symbolic := range []string{"~", "$HOME", "${HOME}"} {
		if cleaned == symbolic || cleaned == symbolic+"/*" || cleaned == symbolic+"/**" {
			return true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return false
	}
	cleanHome := path.Clean(strings.ReplaceAll(home, "\\", "/"))
	return cleaned == cleanHome || cleaned == cleanHome+"/*" || cleaned == cleanHome+"/**"
}
