package tracksession

import (
	"os"
	"syscall"
)

// A kernel-held lease survives suspension safely and is released on process
// death. The permanent sidecar avoids stale-PID unlink/recreate races.
func openLease(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err == syscall.Errno(32) {
		return nil, errSessionBusy
	}
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
