//go:build windows

package platform

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsBackendJobOwnsDescendants(t *testing.T) {
	command := exec.Command("node", "-e", `process.stdin.once('data', () => {
const child = require('node:child_process').spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], {stdio: 'ignore'});
console.log(child.pid);
}); setInterval(() => {}, 1000);`)
	configureBackendProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	terminate, err := ownBackendProcess(command.Process)
	if err != nil {
		t.Fatal(err)
	}
	terminate = sync.OnceFunc(terminate)
	t.Cleanup(terminate)
	if _, err := fmt.Fprintln(stdin, "start"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	terminate()
	status, err := windows.WaitForSingleObject(child, 1000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("owned backend descendant survived job closure: %d %v", status, err)
	}
}
