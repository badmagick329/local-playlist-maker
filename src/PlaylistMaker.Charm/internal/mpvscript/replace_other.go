//go:build !windows

package mpvscript

import "os"

func replaceFile(source, destination string) error { return os.Rename(source, destination) }
