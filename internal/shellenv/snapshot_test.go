package shellenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMarkerPayloadIgnoresShellStartupNoise(t *testing.T) {
	t.Parallel()
	begin := []byte("\x00begin\x00")
	end := []byte("\x00end\x00")
	got, err := markerPayload([]byte("motd\n\x00begin\x00PATH=/bin\x00HOME=/tmp\x00\x00end\x00prompt"), begin, end)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PATH=/bin\x00HOME=/tmp\x00" {
		t.Fatalf("payload = %q", got)
	}
}

func TestParseNULSnapshotSanitizesShellInjectionVariables(t *testing.T) {
	t.Parallel()
	got, err := parseNULSnapshot([]byte("PATH=/custom/bin:/usr/bin\x00HOME=/home/user\x00BASH_ENV=/tmp/evil\x00BASH_FUNC_cat%%=() { echo bad; }\x00PWD=/tmp\x00"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{"BASH_ENV=", "BASH_FUNC_", "PWD="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("snapshot retained %q: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/custom/bin:/usr/bin") {
		t.Fatalf("snapshot lost PATH: %s", joined)
	}
}

func TestCaptureLoadsLoginShellExportedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell test")
	}
	t.Parallel()
	home := t.TempDir()
	shell := filepath.Join(home, "fake-shell")
	script := "#!/bin/sh\nexport DENOVA_CAPTURED=from-profile\nexec /bin/sh \"$@\"\n"
	if err := os.WriteFile(shell, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := capture(t.Context(), shell, home, []string{"PATH=/usr/bin:/bin", "HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnvironment(snapshot.Environment, "DENOVA_CAPTURED=from-profile") {
		t.Fatalf("captured environment = %v", snapshot.Environment)
	}
}

func TestCaptureLoadsExportedZshenvWithoutExecutingCommandsInBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell test")
	}
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is unavailable")
	}
	home := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(home, ".zshenv"),
		[]byte("export DENOVA_ZSHENV_VALUE=from-zshenv\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := capture(t.Context(), zsh, home, []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnvironment(snapshot.Environment, "DENOVA_ZSHENV_VALUE=from-zshenv") {
		t.Fatalf(".zshenv export was not captured: %v", snapshot.Environment)
	}
}

func TestProcessSnapshotReturnsCopy(t *testing.T) {
	t.Parallel()
	source := []string{"PATH=/bin", "VALUE=one", "VALUE=two", "SHELLOPTS=xtrace", "BASHOPTS=extdebug"}
	got := normalizedEnvironment(source)
	if !containsEnvironment(got, "VALUE=two") {
		t.Fatalf("environment = %v", got)
	}
	for _, forbidden := range []string{"SHELLOPTS=", "BASHOPTS="} {
		if strings.Contains(strings.Join(got, "\n"), forbidden) {
			t.Fatalf("environment retained %q: %v", forbidden, got)
		}
	}
}

func containsEnvironment(environment []string, want string) bool {
	for _, entry := range environment {
		if entry == want {
			return true
		}
	}
	return false
}
