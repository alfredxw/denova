//go:build !windows

package restart

import "syscall"

// ReplaceProcess atomically replaces the current process on Unix-like systems.
func ReplaceProcess(invocation Invocation) error {
	args := invocation.Args
	if len(args) == 0 {
		args = []string{invocation.Executable}
	}
	return syscall.Exec(invocation.Executable, args, invocation.Env)
}
