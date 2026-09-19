package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in integration test needs PowerShell 7 and a configured WSL distribution.
func TestWindowsShellExecution(t *testing.T) {
	if os.Getenv("DENOVA_TEST_WINDOWS_SHELLS") != "1" {
		t.Skip("set DENOVA_TEST_WINDOWS_SHELLS=1 to verify installed Windows shells")
	}
	for _, shell := range []string{"pwsh.exe", "wsl.exe"} {
		t.Run(shell, func(t *testing.T) {
			cwd := filepath.Join(t.TempDir(), "workspace with spaces 中文")
			if err := os.Mkdir(cwd, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte("ok"), 0600); err != nil {
				t.Fatal(err)
			}
			command := `if (Test-Path -LiteralPath './sentinel.txt') { Write-Output ('denova-' + 'cwd-ok') }; exit`
			args := []string{"-NoProfile", "-Command", command}
			if shell == "wsl.exe" {
				command = `test -f ./sentinel.txt && printf 'denova-%s\n' 'cwd-ok'; exit`
				args = []string{"--exec", "sh", "-c", command}
			}
			manager := NewManager(Config{Enabled: true, Shell: shell})
			t.Cleanup(manager.CloseAll)
			session, err := manager.Create(Spec{Cwd: cwd, Args: args})
			if err != nil {
				t.Fatal(err)
			}
			if session.Info().Command != shell {
				t.Fatalf("shell fell back: %+v", session.Info())
			}
			history, output, detach := session.Attach(16)
			defer detach()
			var captured strings.Builder
			captured.Write(history)
			if strings.Contains(string(history), "\x1b[6n") {
				_ = session.Write([]byte("\x1b[1;1R"))
			}
			deadline := time.After(15 * time.Second)
			for !strings.Contains(captured.String(), "denova-cwd-ok") {
				select {
				case chunk, ok := <-output:
					captured.Write(chunk)
					// ConPTY shells can query cursor position before reading input.
					if strings.Contains(string(chunk), "\x1b[6n") {
						_ = session.Write([]byte("\x1b[1;1R"))
					}
					if !ok {
						t.Fatalf("shell exited before command succeeded: %q", captured.String())
					}
				case <-deadline:
					t.Fatalf("shell did not execute in project directory: %q", captured.String())
				}
			}
		})
	}
}
