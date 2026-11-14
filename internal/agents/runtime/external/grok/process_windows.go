package grok

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

// windowsMaxCommandLine is CreateProcessW's upper bound on the whole command
// line, measured in UTF-16 code units. The limit includes the terminating null
// character, so the content proper may hold at most windowsMaxCommandLine-1
// code units:
// https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw
const windowsMaxCommandLine = 32_767

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// verifyCommandLine rejects a command line before the OS does. Go builds the
// Windows command line by space-joining escape args; if a turn's flags alone
// exceed the CreateProcessW limit, the process fails with an unreadable error
// instead of a location in the product code. Images never reach this check:
// they now travel inside the .json prompt file. The estimate counts the
// executable as passed in; Go may resolve an extensionless argv0 to ".exe", a
// difference of at most a few code units, which errs toward rejecting rather
// than letting the OS fail.
func verifyCommandLine(executable string, args []string) error {
	escaped := make([]string, 0, len(args)+1)
	escaped = append(escaped, syscall.EscapeArg(executable))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	commandLine := strings.Join(escaped, " ")
	units := len(utf16.Encode([]rune(commandLine)))
	if units < windowsMaxCommandLine {
		return nil
	}
	return fmt.Errorf("Grok command line would reach the Windows %d-character limit (%d UTF-16 code units, null terminator included); shorten the system prompt or instructions, or reduce the number of image attachments before starting the attempt", windowsMaxCommandLine, units)
}