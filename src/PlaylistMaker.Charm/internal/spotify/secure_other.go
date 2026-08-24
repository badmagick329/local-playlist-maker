//go:build !windows

package spotify

import "os"

func secureFile(path string) error { return os.Chmod(path, 0o600) }
