//go:build windows

package update

import "syscall"

const (
	processSynchronize = 0x00100000
	waitTimeout        = 0x00000102
)

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processSynchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	status, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && status == waitTimeout
}
