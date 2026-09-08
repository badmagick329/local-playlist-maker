//go:build !windows

package tracksession

import "os"

func openLockReader(path string) (*os.File, error) { return os.Open(path) }
