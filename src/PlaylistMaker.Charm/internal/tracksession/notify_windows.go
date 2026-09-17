//go:build windows

package tracksession

import (
	"context"
	"os/exec"
	"time"
)

// Notification delivery is best effort; the on-disk hold remains authoritative.
func NotifyTrackingFailure() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $notice = New-Object System.Windows.Forms.NotifyIcon; $notice.Icon = [System.Drawing.SystemIcons]::Warning; $notice.Visible = $true; $notice.ShowBalloonTip(5000, 'PlaylistMaker tracking paused', 'Video is held. Press Play in mpv to retry, or Ctrl+Shift+U to continue without tracking.', [System.Windows.Forms.ToolTipIcon]::Warning); Start-Sleep -Seconds 6; $notice.Dispose()`)
		configureHidden(command)
		_ = command.Run()
	}()
}
