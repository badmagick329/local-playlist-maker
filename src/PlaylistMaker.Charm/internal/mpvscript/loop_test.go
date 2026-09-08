package mpvscript

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/history"
)

// Real mpv events matter here: loop-file does not emit file-loaded or end-file.
func TestMPVRepeatedPlays(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	for _, loop := range []string{"--loop-file=2", "--loop-file=inf", "--loop-playlist=3", "--loop-file=no"} {
		t.Run(loop, func(t *testing.T) {
			want := 3
			if loop == "--loop-file=no" {
				want = 1
			}
			dir := t.TempDir()
			script, err := Ensure(dir)
			if err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(dir, "manifest.json")
			events := filepath.Join(dir, "events.jsonl")
			historyPath := filepath.Join(dir, "history.jsonl")
			if err := os.WriteFile(manifest, []byte(`{"sessionId":"test","entries":[{"entryId":"entry","playlistPosition":0,"videoPath":"test.wav","track":{"trackId":"track"}}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			// Two seconds of silent PCM avoids codecs, downloads and audible output.
			var wav bytes.Buffer
			wav.WriteString("RIFF")
			binary.Write(&wav, binary.LittleEndian, uint32(32000+36))
			wav.WriteString("WAVEfmt ")
			for _, value := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
				binary.Write(&wav, binary.LittleEndian, value)
			}
			wav.WriteString("data")
			binary.Write(&wav, binary.LittleEndian, uint32(32000))
			wav.Write(make([]byte, 32000))
			media := filepath.Join(dir, "test.wav")
			if err := os.WriteFile(media, wav.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--no-config", "--ao=null", "--audio-buffer=0.01", "--ao-null-buffer=0.05", "--vo=null", "--script=" + script, loop,
				"--script-opt=playlistmaker_history-manifest_path=" + manifest,
				"--script-opt=playlistmaker_history-event_path=" + events,
				"--script-opt=playlistmaker_history-history_path=" + historyPath}
			if loop == "--loop-file=inf" {
				stop := filepath.Join(dir, "stop.lua")
				if err := os.WriteFile(stop, []byte(`local n = 0; mp.register_event("playback-restart", function() n = n + 1; if n == 3 then mp.set_property("loop-file", "no") end end)`), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--script="+stop)
			}
			if loop == "--loop-file=no" {
				stop := filepath.Join(dir, "quit-after-half.lua")
				if err := os.WriteFile(stop, []byte(`mp.register_event("file-loaded", function() mp.add_timeout(1.1, function() mp.commandv("quit") end) end)`), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--script="+stop)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, mpv, append(args, media)...).CombinedOutput()
			if err != nil {
				t.Fatalf("mpv: %v\n%s", err, output)
			}
			index, err := history.Read(historyPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := index.Tracks["track"].Played; got != want {
				contents, _ := os.ReadFile(historyPath)
				t.Fatalf("plays = %d, want %d\n%s\n%s", got, want, contents, output)
			}
			contents, err := os.ReadFile(events)
			if err != nil {
				t.Fatal(err)
			}
			starts := bytes.Count(contents, []byte(`"file-loaded"`)) + bytes.Count(contents, []byte(`"playback-repeat"`))
			if starts != want {
				t.Fatalf("tracking starts = %d, want %d\n%s", starts, want, contents)
			}
			if loop == "--loop-file=no" {
				completedQuit := false
				for _, line := range bytes.Split(contents, []byte("\n")) {
					var event struct {
						Event     string `json:"event"`
						Completed bool   `json:"completed"`
						Reason    string `json:"endReason"`
					}
					if json.Unmarshal(line, &event) == nil && event.Event == "end-file" && event.Completed && event.Reason == "quit" {
						completedQuit = true
					}
				}
				if !completedQuit {
					t.Fatalf("quit after half watched did not allow tracking to finish: %s\n%s", contents, output)
				}
			}
		})
	}
}

func TestMPVPausesWhenTrackingStops(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	dir := t.TempDir()
	script, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	events := filepath.Join(dir, "events.jsonl")
	pause := filepath.Join(dir, "tracking-stopped")
	marker := filepath.Join(dir, "paused")
	manifestContents, _ := json.Marshal(map[string]any{
		"sessionId": "test", "pausePath": pause,
		"entries": []map[string]any{{"entryId": "entry", "playlistPosition": 0, "videoPath": "test.wav", "track": map[string]any{"trackId": "track"}}},
	})
	if err := os.WriteFile(manifest, manifestContents, 0o600); err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(48000+36))
	wav.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
		binary.Write(&wav, binary.LittleEndian, value)
	}
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(48000))
	wav.Write(make([]byte, 48000))
	media := filepath.Join(dir, "test.wav")
	if err := os.WriteFile(media, wav.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "pause-probe.lua")
	probeSource := `local mp = require("mp")
local options = require("mp.options")
local config = { marker_path = "" }
options.read_options(config, "pause_probe")
mp.observe_property("pause", "bool", function(_, paused)
    if paused and config.marker_path ~= "" then
        local file = io.open(config.marker_path, "w")
        if file then file:write("paused"); file:close() end
        mp.commandv("quit")
    end
end)`
	if err := os.WriteFile(probe, []byte(probeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, mpv,
		"--no-config", "--ao=null", "--vo=null", "--script="+script, "--script="+probe,
		"--script-opt=playlistmaker_history-manifest_path="+manifest,
		"--script-opt=playlistmaker_history-event_path="+events,
		"--script-opt=pause_probe-marker_path="+marker, media)
	var output bytes.Buffer
	command.Stdout, command.Stderr = io.Discard, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	for {
		contents, _ := os.ReadFile(events)
		if bytes.Contains(contents, []byte(`"file-loaded"`)) {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("mpv did not load the synthetic clip")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(pause, []byte("tracking failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("mpv pause probe: %v\n%s", err, output.Bytes())
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != "paused" {
		t.Fatalf("mpv did not observe the tracking pause: %q, %v", contents, err)
	}
}
