//go:build windows

package app

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func nvidiaSMI() string {
	if path, err := exec.LookPath("nvidia-smi.exe"); err == nil {
		return path
	}
	for _, path := range []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
		filepath.Join(os.Getenv("ProgramW6432"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	paths, _ := filepath.Glob(filepath.Join(os.Getenv("SystemRoot"), "System32", "DriverStore", "FileRepository", "nv*.inf_amd64_*", "nvidia-smi.exe"))
	for i := len(paths) - 1; i >= 0; i-- {
		if info, err := os.Stat(paths[i]); err == nil && !info.IsDir() {
			return paths[i]
		}
	}
	return "nvidia-smi.exe"
}

func setProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func terminateProcess(c *exec.Cmd) { _ = c.Process.Kill() }
func vcRuntimeAvailable() bool {
	for _, name := range []string{"vcruntime140.dll", "vcruntime140_1.dll"} {
		h, err := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		if err != nil {
			return false
		}
		_ = windows.FreeLibrary(h)
	}
	return true
}
func memoryInfo() (uint64, uint64) {
	var m struct {
		Length, Load                                                                                  uint32
		Total, Available, PageTotal, PageAvailable, VirtualTotal, VirtualAvailable, ExtendedAvailable uint64
	}
	m.Length = uint32(unsafe.Sizeof(m))
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&m))); ok != 0 {
		return m.Total, m.Available
	}
	return 0, 0
}
func ownProcess(c *exec.Cmd) (func(), error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	process, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	defer windows.CloseHandle(process)
	if e = windows.AssignProcessToJobObject(job, process); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	return func() { windows.CloseHandle(job) }, nil
}
