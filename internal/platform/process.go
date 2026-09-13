package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type backendProcess struct {
	command   *exec.Cmd
	baseURL   string
	token     string
	stdin     io.WriteCloser
	done      chan struct{}
	once      sync.Once
	tempDir   string
	terminate func()
}

func startBackend(runtime *Runtime, owner *activation) (*backendProcess, error) {
	backend := owner.release.Manifest.backend()
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, failure("NOT_CONFIGURED", "Node.js is required to run %s", owner.release.Manifest.ID)
	}
	tempDir, err := os.MkdirTemp("", "denova-runtime-")
	if err != nil {
		return nil, err
	}
	process := &backendProcess{token: randomToken(), done: make(chan struct{}), tempDir: tempDir}
	entry := filepath.Join(runtime.manager.releasePath(owner.release.Ref), filepath.FromSlash(backend.Launch.Entry))
	process.command = exec.Command(node, append([]string{entry}, backend.Launch.Args...)...)
	process.command.Dir = runtime.manager.releasePath(owner.release.Ref)
	// Do not implicitly pass host model credentials to third-party processes.
	for _, name := range []string{"PATH", "Path", "SystemRoot", "WINDIR", "HOME", "USERPROFILE", "TEMP", "TMP", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(name); ok {
			process.command.Env = append(process.command.Env, name+"="+value)
		}
	}
	configureBackendProcess(process.command)
	stdin, err := process.command.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	process.stdin = stdin
	stdout, err := process.command.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	stderr, err := process.command.StderrPipe()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	if err := process.command.Start(); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, failure("RUNTIME_FAILED", "Start backend %s: %v", owner.release.Manifest.ID, err)
	}
	terminate, err := ownBackendProcess(process.command.Process)
	if err != nil {
		_ = process.command.Process.Kill()
		_ = process.command.Wait()
		_ = os.RemoveAll(tempDir)
		return nil, failure("RUNTIME_FAILED", "Own backend process tree: %v", err)
	}
	process.terminate = sync.OnceFunc(terminate)
	started := false
	defer func() {
		if !started {
			process.close()
		}
	}()
	bootstrap := struct {
		Type       string         `json:"type"`
		Protocol   string         `json:"protocol"`
		Context    RuntimeContext `json:"context"`
		Connection Connection     `json:"connection"`
		PackageDir string         `json:"packageDir"`
		DataDir    string         `json:"dataDir"`
		TempDir    string         `json:"tempDir"`
		HostToken  string         `json:"hostToken"`
	}{"bootstrap", "denova-runtime-v1", owner.context, owner.connection, process.command.Dir, owner.dataDir, tempDir, process.token}
	ready := make(chan struct {
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Type     string `json:"type"`
	}, 1)
	finishedReading := make(chan struct{})
	launch("platform_backend_stdout", runtime.cancel, func() {
		defer close(finishedReading)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		first := true
		for scanner.Scan() {
			if first {
				first = false
				var response struct {
					Port     int    `json:"port"`
					Protocol string `json:"protocol"`
					Type     string `json:"type"`
				}
				if json.Unmarshal(scanner.Bytes(), &response) == nil {
					ready <- response
				}
				continue
			}
			logBackendLine(owner, process.token, scanner.Text())
		}
	})
	launch("platform_backend_stderr", runtime.cancel, func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			logBackendLine(owner, process.token, scanner.Text())
		}
	})
	launch("platform_backend_wait", runtime.cancel, func() {
		defer close(process.done)
		err := process.command.Wait()
		process.terminate()
		if runtime.ctx.Err() == nil {
			slog.Error("platform_backend_exited", "package", owner.release.Manifest.ID, "error", err)
			runtime.cancel()
		}
	})
	if err := json.NewEncoder(stdin).Encode(bootstrap); err != nil {
		return nil, err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case response := <-ready:
		if response.Type != "ready" || response.Protocol != "denova-runtime-v1" || response.Port < 1 || response.Port > 65535 {
			return nil, failure("RUNTIME_FAILED", "Invalid readiness response from %s", owner.release.Manifest.ID)
		}
		process.baseURL = fmt.Sprintf("http://127.0.0.1:%d", response.Port)
	case <-finishedReading:
		return nil, failure("RUNTIME_FAILED", "Backend %s exited before readiness", owner.release.Manifest.ID)
	case <-runtime.ctx.Done():
		return nil, failure("RUNTIME_FAILED", "Backend startup was cancelled")
	case <-timer.C:
		return nil, failure("RUNTIME_FAILED", "Backend %s did not become ready within 15 seconds", owner.release.Manifest.ID)
	}
	ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", process.baseURL+"/__denova/ready", nil)
	request.Header.Set("Authorization", "Bearer "+process.token)
	response, err := localHTTPClient.Do(request)
	if err != nil {
		return nil, failure("RUNTIME_FAILED", "Verify backend readiness: %v", err)
	}
	defer response.Body.Close()
	proof, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil || response.StatusCode != 200 || !bytes.Equal(proof, []byte("denova-runtime-v1")) {
		return nil, failure("RUNTIME_FAILED", "Backend readiness verification failed")
	}
	started = true
	return process, nil
}

func logBackendLine(owner *activation, hostToken, line string) {
	line = strings.ReplaceAll(line, hostToken, "[redacted]")
	line = strings.ReplaceAll(line, owner.connection.Token, "[redacted]")
	slog.Info("platform_backend_log", "package", owner.release.Manifest.ID, "scope", owner.context.Scope, "message", line)
}

func (process *backendProcess) close() {
	process.once.Do(func() {
		if process.stdin != nil {
			_, _ = io.WriteString(process.stdin, "{\"type\":\"shutdown\"}\n")
			_ = process.stdin.Close()
		}
		select {
		case <-process.done:
		case <-time.After(2 * time.Second):
		}
		process.terminate()
		select {
		case <-process.done:
		case <-time.After(3 * time.Second):
			slog.Error("platform_backend_cleanup_timeout", "pid", process.command.Process.Pid)
		}
		_ = os.RemoveAll(process.tempDir)
	})
}

var localHTTPClient = &http.Client{
	Transport:     &http.Transport{Proxy: nil, ResponseHeaderTimeout: 0},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
