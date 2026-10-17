//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const appUserModelID = "Phlo.Panel"

var (
	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	ole32                    = windows.NewLazySystemDLL("ole32.dll")
	procSetAppUserModelID    = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	procSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	procSHChangeNotify       = shell32.NewProc("SHChangeNotify")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	procCoUninitialize       = ole32.NewProc("CoUninitialize")
	procCoCreateInstance     = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

func setAppUserModelID() {
	id, err := windows.UTF16PtrFromString(appUserModelID)
	if err != nil {
		return
	}
	procSetAppUserModelID.Call(uintptr(unsafe.Pointer(id)))
}

func ensureDesktopShortcut(root string) {
	target := filepath.Join(root, "bin", "phlo-panel.exe")
	if _, err := os.Stat(target); err != nil {
		target = filepath.Join(root, "Phlo.vbs")
		if _, err := os.Stat(target); err != nil {
			return
		}
	}
	icon := filepath.Join(root, "ui", "logo.ico")
	if _, err := os.Stat(icon); err != nil {
		return
	}
	desktop, err := desktopDir()
	if err != nil || desktop == "" {
		return
	}
	linkPath := filepath.Join(desktop, "Phlo.lnk")
	if err := writeShortcut(linkPath, target, root, icon); err != nil {
		return
	}
	path, err := windows.UTF16PtrFromString(linkPath)
	if err != nil {
		return
	}
	procSHChangeNotify.Call(0x00002000, 0x0005, uintptr(unsafe.Pointer(path)), 0)
}

func desktopDir() (string, error) {
	fid := windows.GUID{
		Data1: 0xB4BFCC3A,
		Data2: 0xDB2C,
		Data3: 0x424C,
		Data4: [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41},
	}
	var path *uint16
	hr, _, _ := procSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&fid)), 0, 0, uintptr(unsafe.Pointer(&path)))
	if int32(hr) < 0 || path == nil {
		return "", syscall.Errno(hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(path)))
	return windows.UTF16PtrToString(path), nil
}

func writeShortcut(linkPath, target, work, icon string) error {
	hr, _, _ := procCoInitializeEx.Call(0, 2)
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	defer procCoUninitialize.Call()

	clsid := windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidLink := windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	var link uintptr
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)),
		0,
		1,
		uintptr(unsafe.Pointer(&iidLink)),
		uintptr(unsafe.Pointer(&link)),
	)
	if int32(hr) < 0 || link == 0 {
		return syscall.Errno(hr)
	}
	defer comCall(link, 2)

	if err := setWide(link, 20, target); err != nil {
		return err
	}
	if err := setWide(link, 9, work); err != nil {
		return err
	}
	if err := setWide(link, 7, "Phlo"); err != nil {
		return err
	}
	iconPtr, err := windows.UTF16PtrFromString(icon)
	if err != nil {
		return err
	}
	if failed(comCall(link, 17, uintptr(unsafe.Pointer(iconPtr)), 0)) {
		return syscall.EINVAL
	}
	if failed(comCall(link, 15, 1)) {
		return syscall.EINVAL
	}
	setShortcutAppID(link)

	iidFile := windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	var file uintptr
	if failed(comCall(link, 0, uintptr(unsafe.Pointer(&iidFile)), uintptr(unsafe.Pointer(&file)))) || file == 0 {
		return syscall.EINVAL
	}
	defer comCall(file, 2)
	name, err := windows.UTF16PtrFromString(linkPath)
	if err != nil {
		return err
	}
	if failed(comCall(file, 6, uintptr(unsafe.Pointer(name)), 1)) {
		return syscall.EINVAL
	}
	return nil
}

func setShortcutAppID(link uintptr) {
	iidStore := windows.GUID{
		Data1: 0x886D8EEB,
		Data2: 0x8D3D,
		Data3: 0x4BC3,
		Data4: [8]byte{0x80, 0xE4, 0x14, 0xFD, 0xA1, 0x2C, 0xD3, 0xA8},
	}
	var store uintptr
	if failed(comCall(link, 0, uintptr(unsafe.Pointer(&iidStore)), uintptr(unsafe.Pointer(&store)))) || store == 0 {
		return
	}
	defer comCall(store, 2)
	key := struct {
		Fmtid windows.GUID
		Pid   uint32
	}{
		Fmtid: windows.GUID{
			Data1: 0x9F4C2855,
			Data2: 0x9F79,
			Data3: 0x4B39,
			Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3},
		},
		Pid: 5,
	}
	id, err := windows.UTF16PtrFromString(appUserModelID)
	if err != nil {
		return
	}
	value := struct {
		VT      uint16
		R1      uint16
		R2      uint16
		R3      uint16
		Pointer uintptr
		Pad     uint64
	}{VT: 31, Pointer: uintptr(unsafe.Pointer(id))}
	if failed(comCall(store, 6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value)))) {
		return
	}
	comCall(store, 7)
}

func setWide(obj uintptr, slot int, value string) error {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return err
	}
	if failed(comCall(obj, slot, uintptr(unsafe.Pointer(ptr)))) {
		return syscall.EINVAL
	}
	return nil
}

func comCall(unk uintptr, slot int, args ...uintptr) uintptr {
	vt := *(*uintptr)(unsafe.Pointer(unk))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, 1+len(args))
	all = append(all, unk)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func failed(hr uintptr) bool {
	return int32(hr) < 0
}
