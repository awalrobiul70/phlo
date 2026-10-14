//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func Configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil || handle == 0 {
		return false
	}
	_ = syscall.CloseHandle(handle)
	return true
}

func Stop(pid int) error {
	if pid <= 0 || !Alive(pid) {
		return nil
	}
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	Configure(cmd)
	return cmd.Run()
}
