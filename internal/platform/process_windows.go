package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObject    = modkernel32.NewProc("CreateJobObjectW")
	procAssignProcess      = modkernel32.NewProc("AssignProcessToJobObject")
	procTerminateJob       = modkernel32.NewProc("TerminateJobObject")
	procCloseHandle        = modkernel32.NewProc("CloseHandle")
	procOpenProcess        = modkernel32.NewProc("OpenProcess")
	procSetInformation     = modkernel32.NewProc("SetInformationJobObject")
	procGetExitCodeProcess = modkernel32.NewProc("GetExitCodeProcess")
)

const (
	PROCESS_CREATE_PROCESS             = 0x0080
	PROCESS_TERMINATE                  = 0x0001
	PROCESS_QUERY_INFORMATION          = 0x0400
	SYNCHRONIZE                        = 0x00100000
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000
	JobObjectExtendedLimitInformation  = 9
)

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation struct {
		PerProcessUserTimeLimit int64
		PerJobUserTimeLimit     int64
		LimitFlags              uint32
		MinimumWorkingSetSize   uintptr
		MaximumWorkingSetSize   uintptr
		ActiveProcessLimit      uint32
		Affinity                uintptr
		ChildProcessRateControl uint32
		_                       [4]byte
	}
	IoInfo struct {
		ReadOperationCount  int64
		WriteOperationCount int64
		OtherOperationCount int64
		ReadTransferCount   int64
		WriteTransferCount  int64
		OtherTransferCount  int64
	}
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func KillProcessTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}

	// Try job object approach first (most thorough)
	procHandle, _, _ := procOpenProcess.Call(
		PROCESS_CREATE_PROCESS|PROCESS_TERMINATE|PROCESS_QUERY_INFORMATION|SYNCHRONIZE,
		0,
		uintptr(pid),
	)
	if procHandle != 0 {
		defer procCloseHandle.Call(procHandle)

		jobHandle, _, _ := procCreateJobObject.Call(0, 0)
		if jobHandle != 0 {
			defer procCloseHandle.Call(jobHandle)

			info := &jobObjectExtendedLimitInformation{}
			info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

			r1, _, _ := procSetInformation.Call(
				jobHandle,
				JobObjectExtendedLimitInformation,
				uintptr(unsafe.Pointer(info)),
				unsafe.Sizeof(*info),
			)
			if r1 != 0 {
				r1, _, _ = procAssignProcess.Call(jobHandle, procHandle)
				if r1 != 0 {
					r1, _, _ = procTerminateJob.Call(jobHandle, 1)
					if r1 != 0 {
						return nil
					}
				}
			}
		}
	}

	// Fallback: use taskkill /F /T to force-kill the process tree
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

func SetProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}

func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, _, _ := procOpenProcess.Call(PROCESS_QUERY_INFORMATION, 0, uintptr(pid))
	if handle == 0 {
		return false
	}
	defer procCloseHandle.Call(handle)
	var exitCode uint32
	ret, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	if ret == 0 {
		return false
	}
	return exitCode == 259 // STILL_ACTIVE
}

func FindExecutable(name string) string {
	if name == "" {
		return ""
	}

	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	commonPaths := []string{
		filepath.Join("C:\\", "Program Files", name, name+".exe"),
		filepath.Join("C:\\", "Program Files (x86)", name, name+".exe"),
		filepath.Join("C:\\", "Program Files", "ffmpeg", "bin", name+".exe"),
		filepath.Join("C:\\", "Program Files (x86)", "ffmpeg", "bin", name+".exe"),
		filepath.Join("C:\\", "ffmpeg", "bin", name+".exe"),
		filepath.Join("C:\\", "tools", name+".exe"),
	}

	for _, path := range commonPaths {
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
	}

	return ""
}
