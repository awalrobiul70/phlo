//go:build windows

package terminal

import (
	"os/exec"
	"syscall"
)

func spawn(title, dir string, command []string) (*exec.Cmd, error) {
	return windowsCommand(look("wt.exe", "wt"), title, dir, command), nil
}

func windowsCommand(wt, title, dir string, command []string) *exec.Cmd {
	if wt != "" {
		args := []string{"-w", "0", "new-tab", "--title", title, "-d", dir}
		if len(command) == 0 {
			args = append(args, "cmd.exe")
		} else {
			args = append(args, "cmd.exe", "/k")
			args = append(args, command...)
		}
		cmd := exec.Command(wt, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
		return cmd
	}
	args := []string{"/c", "start", title, "/D", dir}
	if len(command) == 0 {
		args = append(args, "cmd.exe")
	} else {
		args = append(args, "cmd.exe", "/k")
		args = append(args, command...)
	}
	cmd := exec.Command("cmd.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}
