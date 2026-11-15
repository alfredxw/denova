//go:build integration

package update

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Run on native Windows, Linux and macOS; cross compilation cannot exercise file
// locks, process handles, executable replacement or the shutdown handoff.
func TestUpdaterProcessHandoffAndRecovery(t *testing.T) {
	root := t.TempDir()
	exeName := "denova"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	fixture := filepath.Join(root, "fixture"+filepath.Ext(exeName))
	updater := filepath.Join(root, updaterExecutableName())
	for _, build := range []struct{ target, pkg string }{{fixture, "./testdata/app"}, {updater, "../../cmd/denova-updater"}} {
		cmd := exec.Command("go", "build", "-ldflags", "-X denova/internal/buildinfo.Version=0.2.0", "-o", build.target, build.pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build process fixture: %v\n%s", err, output)
		}
	}
	for _, scenario := range []struct {
		name          string
		phase         Phase
		move          bool
		legacy        bool
		legacyDefault bool
	}{
		{name: "normal", phase: PhaseStaged},
		{name: "interrupted", phase: PhaseApplying},
		{name: "moved staged", phase: PhaseStaged, move: true},
		{name: "moved completed", phase: PhaseSucceeded, move: true},
		{name: "moved interrupted", phase: PhaseApplying, move: true},
		{name: "v0.5.1 handoff", phase: PhaseStaged, legacy: true},
		{name: "v0.5.1 default port", phase: PhaseStaged, legacy: true, legacyDefault: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.legacyDefault {
				// This regression specifically needs the released default address.
				// Never interfere with another application's listener on that port.
				probe, err := net.Listen("tcp", "127.0.0.1:8080")
				if err != nil {
					t.Skipf("legacy default port is occupied: %v", err)
				}
				_ = probe.Close()
			}
			recovering := scenario.phase == PhaseApplying
			completed := scenario.phase == PhaseSucceeded
			install := t.TempDir()
			source := filepath.Join(updateDataDir(install), "pending-test", "denova")
			writeUpdateTestPackage(t, install, updateTestPackageContents{Skill: "old skill"})
			writeUpdateTestPackage(t, source, updateTestPackageContents{Skill: "new skill"})
			target := filepath.Join(install, exeName)
			for _, dest := range []string{target, filepath.Join(source, exeName)} {
				if err := copyFile(fixture, dest, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := copyFile(updater, filepath.Join(source, updaterExecutableName()), 0o755); err != nil {
				t.Fatal(err)
			}
			m := ApplyManifest{ID: "process-test", State: scenario.phase, Version: "0.2.0", InstallDir: install, SourceDir: source, BackupDir: filepath.Join(filepath.Dir(source), "backup"), TargetExecutable: target, UpdaterExecutable: filepath.Join(source, updaterExecutableName()), LogPath: filepath.Join(filepath.Dir(source), "apply.log")}
			m.RelaunchArgs = []string{target, "--port", "0", "--no-open"}
			path := filepath.Join(filepath.Dir(source), manifestFileName)
			if recovering {
				if err := backupUpdate(&m); err != nil {
					t.Fatal(err)
				}
				// Simulate termination after the new executable was placed but while assets
				// were only partly replaced. Startup must recover before serving any request.
				if err := os.RemoveAll(filepath.Join(install, "skills")); err != nil {
					t.Fatal(err)
				}
				m.State = PhaseApplying
			}
			if err := writeManifest(path, m); err != nil {
				t.Fatal(err)
			}
			if err := writePendingManifestRef(updateDataDir(install), path); err != nil {
				t.Fatal(err)
			}
			if scenario.legacy {
				// v0.5.1 launches the staged helper directly with absolute paths
				// and no transaction state or readiness protocol in the old app.
				legacy := map[string]any{"source_dir": source, "install_dir": install, "backup_dir": filepath.Join(updateDataDir(install), "backup-legacy"), "target_executable": target, "updater_executable": m.UpdaterExecutable, "version": m.Version, "relaunch_args": []string{target, "--port", "0", "--no-open"}, "log_path": m.LogPath}
				if scenario.legacyDefault {
					legacy["relaunch_args"] = []string{target, "--no-open"}
				}
				if err := writeJSONFile(path, legacy, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := writeJSONFile(filepath.Join(updateDataDir(install), pendingManifestRefName), pendingManifestRef{ManifestPath: path}, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.move {
				moved := filepath.Join(t.TempDir(), "portable")
				if err := os.Rename(install, moved); err != nil {
					t.Fatal(err)
				}
				install = moved
				var err error
				path, err = readPendingManifestRef(updateDataDir(install))
				if err != nil {
					t.Fatal(err)
				}
				m, err = readManifest(path)
				if err != nil {
					t.Fatal(err)
				}
				target = m.TargetExecutable
			}
			log, err := os.Create(filepath.Join(install, "process.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			cmd := exec.Command(target, "--port", "0", "--no-open")
			if scenario.legacy {
				cmd = exec.Command(m.UpdaterExecutable, "--manifest", path)
			}
			cmd.Env = append(os.Environ(), "DENOVA_UPDATE_TEST_ROOT="+install)
			cmd.Dir = install
			cmd.Stdout, cmd.Stderr = log, log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Reap immediately on Unix: an exited but unreaped child still
			// answers kill(pid, 0), unlike a terminated Windows process handle.
			finished := make(chan error, 1)
			go func() {
				defer func() {
					if value := recover(); value != nil {
						finished <- fmt.Errorf("wait panicked: %v", value)
					}
				}()
				finished <- cmd.Wait()
			}()
			defer func() {
				_ = cmd.Process.Kill()
				<-finished
				if data, err := os.ReadFile(filepath.Join(install, "pid")); err == nil {
					if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid != cmd.Process.Pid {
						if process, err := os.FindProcess(pid); err == nil {
							_ = process.Kill()
							_ = process.Release()
						}
					}
				}
				if t.Failed() {
					for _, diagnostic := range []string{filepath.Join(install, "process.log"), m.LogPath, path, path + ".accepted"} {
						data, err := os.ReadFile(diagnostic)
						t.Logf("%s: %s (read error: %v)", diagnostic, data, err)
					}
				}
			}()
			client := &http.Client{Timeout: time.Second}
			defer client.CloseIdleConnections()
			deadline := time.Now().Add(20 * time.Second)
			baseURL := ""
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(filepath.Join(install, "port"))
				if err == nil {
					baseURL = "http://127.0.0.1:" + string(data)
					if resp, err := client.Get(baseURL + "/api/update/status"); err == nil {
						_ = resp.Body.Close()
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			if baseURL == "" {
				t.Fatal("app never started")
			}
			if scenario.legacyDefault && baseURL != "http://127.0.0.1:8080" {
				t.Fatalf("legacy browser origin changed after update: %s", baseURL)
			}
			if !recovering && !completed && !scenario.legacy {
				// Applying waits for a real child to acknowledge handoff before HTTP
				// returns. Its request budget must exceed the production handshake.
				applyClient := &http.Client{Timeout: applyHandoffTimeout + 5*time.Second}
				defer applyClient.CloseIdleConnections()
				resp, err := applyClient.Post(baseURL+"/apply", "application/json", nil)
				if err != nil {
					t.Fatal(err)
				}
				var applied ApplyResult
				err = json.NewDecoder(resp.Body).Decode(&applied)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 || err != nil || applied.ID != m.ID {
					t.Fatalf("handoff failed: %+v %v HTTP %d", applied, err, resp.StatusCode)
				}
			}
			wantPhase, wantSkill := PhaseSucceeded, "new skill"
			if recovering {
				wantPhase, wantSkill = PhaseFailed, "old skill"
			} else if completed {
				wantSkill = "old skill"
			}
			deadline = time.Now().Add(20 * time.Second)
			for {
				actual, err := readManifest(path)
				if err == nil && actual.State == wantPhase {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("update did not finish: %+v %v", actual, err)
				}
				time.Sleep(50 * time.Millisecond)
			}
			assertFileContent(t, filepath.Join(install, "skills", "demo", "SKILL.md"), wantSkill)
			if !completed {
				assertFileContent(t, filepath.Join(m.BackupDir, "skills", "demo", "SKILL.md"), "old skill")
			}
			if !recovering && !completed && !scenario.legacy {
				assertFileContent(t, filepath.Join(install, "closed"), "closed before exit")
			}
			// Read the running process's status, not merely the updater's receipt.
			resp, err := client.Get(baseURL + "/api/update/status")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var status Status
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil || status.Phase != wantPhase {
				t.Fatalf("running status: %+v %v", status, err)
			}
		})
	}
}
