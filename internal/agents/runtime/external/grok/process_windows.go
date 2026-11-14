package grok

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

// windowsMaxCommandLine is CreateProcessW's upper bound on the whole command
// line, measured in UTF-16 code units (including executable, arguments, spaces
// and quotes):
// https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw
const windowsMaxCommandLine = 32_767

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// verifyCommandLine rejects a command line before the OS does. Go builds the
// Windows command line by space-joining escape args; if a turn's flags alone
// exceed the CreateProcessW limit, the process fails with an unreadable error
// instead of a location in the product code. Images never reach this check:
// they now travel inside the .json prompt file.
func verifyCommandLine(executable string, args []string) error {
	escaped := make([]string, 0, len(args)+1)
	escaped = append(escaped, syscall.EscapeArg(executable))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	commandLine := strings.Join(escaped, " ")
	units := len(utf16.Encode([]rune(commandLine)))
	if units <= windowsMaxCommandLine {
		return nil
	}
	return fmt.Errorf("Grok command line would exceed the Windows %d-character limit (%d UTF-16 code units); shorten the system prompt or instructions before starting the attempt", windowsMaxCommandLine, units)
}