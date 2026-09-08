package tracksession

import (
	"os"
	"syscall"
	"time"
)

// Waiting helpers must not prevent the owner from deleting its lock on Windows.
func openLockReader(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Windows also reports access denied while deletion is pending. Retry that
	// handoff briefly, but still surface persistent permission failures.
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		handle, err := syscall.CreateFile(name, syscall.GENERIC_READ,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
			nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return os.NewFile(uintptr(handle), path), nil
		}
		if err != syscall.ERROR_ACCESS_DENIED || !time.Now().Before(deadline) {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		time.Sleep(time.Millisecond)
	}
}
