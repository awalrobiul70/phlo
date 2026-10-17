//go:build windows

package proc

import (
	"syscall"
	"unsafe"
)

var (
	kernel32Owned = syscall.NewLazyDLL("kernel32.dll")
	procSnapshot  = kernel32Owned.NewProc("CreateToolhelp32Snapshot")
	procFirst     = kernel32Owned.NewProc("Process32FirstW")
	procNext      = kernel32Owned.NewProc("Process32NextW")
	procOpen      = kernel32Owned.NewProc("OpenProcess")
	procImage     = kernel32Owned.NewProc("QueryFullProcessImageNameW")
	procClose     = kernel32Owned.NewProc("CloseHandle")
)

const (
	snapProcess = 0x00000002
	queryLimit  = 0x1000
)

type processEntry struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// OwnedServicePIDs lists service programs started from this Phlo folder.
func OwnedServicePIDs(root string) []int {
	if root == "" {
		return nil
	}
	snap, _, _ := procSnapshot.Call(snapProcess, 0)
	if snap == 0 || snap == ^uintptr(0) {
		return nil
	}
	defer procClose.Call(snap)
	var entry processEntry
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, _ := procFirst.Call(snap, uintptr(unsafe.Pointer(&entry)))
	var pids []int
	for ok != 0 {
		if image := processImage(entry.ProcessID); IsPhloService(root, image) {
			pids = append(pids, int(entry.ProcessID))
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, _ = procNext.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	return pids
}

func processImage(pid uint32) string {
	if pid == 0 {
		return ""
	}
	handle, _, _ := procOpen.Call(queryLimit, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procClose.Call(handle)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := procImage.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}
