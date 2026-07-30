package toolapproval

import (
	"path"
	"strings"
)

// criticalInvocation covers path-qualified and wrapper-unwrapped commands that
// regex rules cannot reliably anchor. It intentionally targets only host-level
// destructive operations; ordinary project scripts remain a Yolo tradeoff.
func criticalInvocation(name string, args []string) *Decision {
	if criticalHostControl(name, args) {
		result := deny("critical_power_control", RiskCritical,
			"禁止关闭、重启宿主系统或终止其初始化进程。 / Host shutdown, reboot, and init-process termination are blocked.")
		return &result
	}
	if criticalDiskCommand(name, args) {
		result := deny("critical_disk_format", RiskCritical,
			"禁止格式化、擦除或重写磁盘。 / Disk formatting, wiping, and partition rewriting are blocked.")
		return &result
	}
	if name == "dd" && criticalDDWrite(args) {
		result := deny("critical_raw_device_write", RiskCritical,
			"禁止向原始块设备写入数据。 / Writes to raw block devices are blocked.")
		return &result
	}
	if criticalAuthenticationMutation(name, args) {
		result := deny("critical_auth_files", RiskCritical,
			"禁止修改系统认证与 sudo 配置。 / Changes to system authentication and sudo configuration are blocked.")
		return &result
	}
	return nil
}

func criticalHostControl(name string, args []string) bool {
	switch name {
	case "shutdown", "reboot", "poweroff", "halt":
		return true
	case "systemctl", "loginctl":
		return containsArgument(args, "poweroff", "reboot", "halt")
	case "init", "telinit":
		return containsArgument(args, "0", "6")
	case "launchctl":
		return containsArgument(args, "reboot")
	case "kill":
		for _, value := range args {
			value = strings.TrimSpace(value)
			if value == "1" || strings.TrimLeft(value, "0") == "1" {
				return true
			}
		}
	case "killall", "pkill":
		return containsArgument(args, "init", "systemd", "launchd")
	}
	return false
}

func criticalDiskCommand(name string, args []string) bool {
	if name == "mkfs" || strings.HasPrefix(name, "mkfs.") || strings.HasPrefix(name, "newfs") {
		return true
	}
	switch name {
	case "wipefs", "fdisk", "cfdisk", "sfdisk", "parted", "sgdisk", "cryptsetup", "blkdiscard":
		return true
	case "diskutil":
		return containsArgument(args, "erasedisk", "partitiondisk", "zerodisk", "secureerase", "deletecontainer")
	case "nvme":
		return containsArgument(args, "format", "sanitize")
	case "hdparm":
		for _, value := range args {
			if strings.HasPrefix(strings.ToLower(value), "--security-erase") {
				return true
			}
		}
	}
	return false
}

func criticalDDWrite(args []string) bool {
	for _, value := range args {
		if target, found := strings.CutPrefix(strings.ToLower(strings.TrimSpace(value)), "of="); found &&
			criticalRawDeviceTarget(target) {
			return true
		}
	}
	return false
}

func criticalAuthenticationMutation(name string, args []string) bool {
	if !oneOf(name, "rm", "mv", "cp", "install", "chmod", "chown", "truncate", "tee", "sed", "ln") {
		return false
	}
	for _, argument := range args {
		value := argument
		if _, optionValue, found := strings.Cut(argument, "="); found {
			value = optionValue
		}
		value = path.Clean(strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), `"'`))
		if value == "/etc/passwd" || value == "/etc/shadow" || value == "/etc/sudoers" ||
			strings.HasPrefix(value, "/etc/sudoers.d/") {
			return true
		}
	}
	return false
}

func criticalRawDeviceTarget(value string) bool {
	value = path.Clean(strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), `"'`))
	return value == "/proc/sysrq-trigger" || rawDevicePathPattern.MatchString(value)
}
