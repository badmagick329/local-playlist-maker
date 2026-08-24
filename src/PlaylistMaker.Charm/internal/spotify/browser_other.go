//go:build !windows

package spotify

import "os/exec"

func openBrowser(target string) error { return exec.Command("xdg-open", target).Start() }
