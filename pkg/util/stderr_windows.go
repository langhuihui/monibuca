//go:build windows

package util

import (
	"log"
	"os"
	"syscall"
)

var (
	kernel32         = syscall.MustLoadDLL("kernel32.dll")
	procSetStdHandle = kernel32.MustFindProc("SetStdHandle")
)

func setStdHandle(stdhandle int32, handle syscall.Handle) error {
	r0, _, e1 := syscall.Syscall(procSetStdHandle.Addr(), 2, uintptr(stdhandle), uintptr(handle), 0)
	if r0 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

// redirectStderr to the file passed in
func init() {
	logFile := initFatalLog()
	if logFile != nil {
		err := setStdHandle(syscall.STD_ERROR_HANDLE, syscall.Handle(logFile.Fd()))
		if err != nil {
			log.Fatalf("Failed to redirect stderr to file: %v", err)
		}
		// SetStdHandle does not affect prior references to stderr
		os.Stderr = logFile
	}
}
