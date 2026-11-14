package grok

import (
	"runtime"
	"strings"
	"testing"
)

func TestVerifyCommandLineAcceptsNormalArgs(t *testing.T) {
	args := []string{
		"--prompt-file", "C:\\home\\prompt.json",
		"--output-format", "streaming-json",
		"--model", "grok-4.7",
		"--always-approve",
		"--tools", "todo_write,search_tool,use_tool",
	}
	if err := verifyCommandLine("C:\\grok.exe", args); err != nil {
		t.Fatalf("normal command line rejected: %v", err)
	}
}

// The prompt payload is a file path, so an oversized image must never surface
// in the command line; the guard only matters for flag bloat such as an
// oversized system prompt override. The Windows implementation rejects an
// oversize command line; other platforms leave it to the OS.
func TestVerifyCommandLineWindowsRejectsOversize(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	args := []string{"--system-prompt-override", strings.Repeat("x", 40_000)}
	err := verifyCommandLine("C:\\grok.exe", args)
	if runtime.GOOS == "windows" && err == nil {
		t.Fatal("oversized command line was accepted on Windows")
	}
	if runtime.GOOS != "windows" && err != nil {
		t.Fatalf("non-Windows platform rejected a long command line: %v", err)
	}
}