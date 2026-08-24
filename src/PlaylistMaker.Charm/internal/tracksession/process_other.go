//go:build !windows

package tracksession

import "os/exec"

func configureHidden(*exec.Cmd) {}
