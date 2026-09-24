package terminal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsDefaultShellPrefersPwshAndIgnoresComspec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)
	if got := platformDefaultShell(); got != "powershell.exe" {
		t.Fatalf("default without pwsh = %q, want Windows PowerShell", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "pwsh.exe"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := platformDefaultShell(); got != "pwsh.exe" {
		t.Fatalf("default with pwsh = %q, want pwsh.exe", got)
	}
}
