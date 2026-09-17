package tracksession

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/mpvscript"
	"playlistmaker/charm/internal/spotify"
	"playlistmaker/charm/internal/tracking"
)

func TestMPVRecoveryEvidenceHoldsUntilProgressOrCompletion(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	for _, scenario := range []string{"early-pause", "short-tail"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			var wav bytes.Buffer
			wav.WriteString("RIFF")
			binary.Write(&wav, binary.LittleEndian, uint32(320000+36))
			wav.WriteString("WAVEfmt ")
			for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
				binary.Write(&wav, binary.LittleEndian, v)
			}
			wav.WriteString("data")
			binary.Write(&wav, binary.LittleEndian, uint32(320000))
			wav.Write(make([]byte, 320000))
			media := filepath.Join(dir, "silent.wav")
			if err = os.WriteFile(media, wav.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			script, err := mpvscript.Ensure(dir)
			if err != nil {
				t.Fatal(err)
			}
			var state spotify.PlaybackState
			var started, resumed time.Time
			starts, resumes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/me/player/devices":
					json.NewEncoder(w).Encode(map[string]any{"devices": []spotify.Device{{ID: "device", Name: "fixture"}}})
				case strings.HasPrefix(r.URL.Path, "/tracks/"):
					json.NewEncoder(w).Encode(spotify.Track{URI: "spotify:track:fixture", DurationMS: 2000})
				case r.Method == http.MethodGet:
					if scenario == "early-pause" {
						if !resumed.IsZero() && time.Since(resumed) >= 2*time.Second {
							state.IsPlaying = true
							state.ProgressMS = 1500
						}
					} else if !resumed.IsZero() {
						if time.Since(resumed) >= 500*time.Millisecond {
							state.IsPlaying = false
							state.ProgressMS = 2000
							state.Timestamp = time.Now().UnixMilli()
						}
					} else if state.IsPlaying {
						state.ProgressMS = min(1500, int(time.Since(started).Milliseconds()))
					}
					json.NewEncoder(w).Encode(state)
				default:
					if r.URL.Path == "/me/player/play" {
						body, _ := io.ReadAll(r.Body)
						if len(body) == 0 {
							resumes++
							resumed = time.Now()
						} else {
							starts++
							started = time.Now()
							state = spotify.PlaybackState{IsPlaying: scenario != "early-pause", RepeatState: "off", Timestamp: started.UnixMilli(), Item: &spotify.Track{URI: "spotify:track:fixture", DurationMS: 2000}}
							state.Device.ID = "device"
							if scenario == "early-pause" {
								state.ProgressMS = 500
							}
						}
					}
					if r.URL.Path == "/me/player/pause" {
						state.IsPlaying = false
						state.Timestamp = time.Now().UnixMilli()
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			authPath := filepath.Join(dir, "auth.json")
			data, _ := json.Marshal(spotify.Token{AccessToken: "synthetic", ExpiresAtUTC: time.Now().Add(time.Hour)})
			if err = os.WriteFile(authPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			path, m, err := Create(dir, []Entry{{VideoPath: media, Track: tracking.Track{SpotifyURI: "spotify:track:fixture"}}}, false, false, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			m.MPVProcessID = 123
			if err = WriteManifest(path, m); err != nil {
				t.Fatal(err)
			}
			player := &spotify.Player{Client: &spotify.Client{Auth: &spotify.Auth{TokenPath: authPath}, HTTP: server.Client(), APIBase: server.URL}, StatePath: m.SpotifyStatePath, SessionID: m.SessionID}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- (Runner{Runtime: &Runtime{Spotify: player}, DeviceName: "fixture", Poll: 10 * time.Millisecond, IsAlive: func(int) bool { return true }}).Run(ctx, path)
			}()
			marker := filepath.Join(dir, "result")
			probe := filepath.Join(dir, "probe.lua")
			source := `local mp=require("mp")
local o={marker="",scenario=""};require("mp.options").read_options(o,"evidence")
local failed=false
local function check(paused,label)
 if mp.get_property_native("pause")~=paused then failed=true;local f=io.open(o.marker,"a");f:write(label .. " failed\n");f:close() end
end
if o.scenario=="early-pause" then
 mp.add_timeout(0.8,function() check(true,"unconfirmed start") end)
 mp.add_timeout(1.6,function() check(true,"accepted resume without progress") end)
 mp.add_timeout(3.5,function() check(false,"advancing confirmation") end)
else
 mp.add_timeout(0.8,function() mp.commandv("script-message","playlistmaker-pause");mp.commandv("seek",1.5,"absolute+exact") end)
 mp.add_timeout(3.2,function() check(true,"deliberate ceiling");mp.commandv("script-message","playlistmaker-retry") end)
 mp.add_timeout(3.5,function() check(true,"unconfirmed short tail") end)
 mp.add_timeout(5.2,function() check(false,"verified stopped endpoint") end)
end
mp.add_timeout(5.5,function() local f=io.open(o.marker,"a");if not failed then f:write("ok") end;f:close();mp.commandv("quit") end)`
			if err = os.WriteFile(probe, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			output, playErr := exec.CommandContext(ctx, mpv, "--no-config", "--ao=null", "--vo=null", "--script="+script, "--script="+probe,
				"--script-opt=playlistmaker_history-manifest_path="+path, "--script-opt=playlistmaker_history-event_path="+m.EventPath,
				"--script-opt=evidence-marker="+marker, "--script-opt=evidence-scenario="+scenario, media).CombinedOutput()
			if playErr != nil {
				cancel()
			}
			runErr := <-done
			result, _ := os.ReadFile(marker)
			if playErr != nil || runErr != nil || string(result) != "ok" {
				t.Fatalf("mpv=%v helper=%v checks=%s\n%s", playErr, runErr, result, output)
			}
			if starts != 1 || resumes != 1 {
				t.Fatalf("occurrence replayed: starts=%d resumes=%d", starts, resumes)
			}
		})
	}
}
