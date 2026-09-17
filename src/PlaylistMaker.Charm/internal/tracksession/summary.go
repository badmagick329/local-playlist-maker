package tracksession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Prioritize sessions needing intervention even when another helper is healthy.
func Summary(directory string, now time.Time) string {
	paths, _ := filepath.Glob(filepath.Join(directory, "tracking-sessions", "*.manifest.json"))
	message := ""
	for _, path := range paths {
		m, err := ReadManifest(path)
		if err != nil || m.ShutdownSeen || m.MPVProcessID == 0 {
			continue
		}
		data, err := os.ReadFile(m.StatusPath)
		if err != nil {
			continue
		}
		var s Status
		if json.Unmarshal(data, &s) != nil || s.SessionID != m.SessionID {
			continue
		}
		if now.Sub(time.UnixMilli(s.Heartbeat)) > 15*time.Second {
			return "Tracking helper unresponsive; mpv held. Play retries; Ctrl+Shift+U continues without tracking."
		}
		if s.Hold {
			return s.Message + " (mpv: Play retries; Ctrl+Shift+U continues without tracking)"
		}
		message = s.Message
	}
	return message
}
