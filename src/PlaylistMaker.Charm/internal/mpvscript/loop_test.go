package mpvscript

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/tracking"
	"playlistmaker/charm/internal/tracksession"
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

func TestMPVPersistentHoldAndUserIntent(t *testing.T) {
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
	status := filepath.Join(dir, "status.json")
	marker := filepath.Join(dir, "result")
	data, _ := json.Marshal(map[string]any{"sessionId": "test", "statusPath": status, "entries": []map[string]any{{"entryId": "entry", "playlistPosition": 0, "track": map[string]any{"trackId": "track"}}}})
	if err = os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(480000+36))
	wav.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
		binary.Write(&wav, binary.LittleEndian, v)
	}
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(480000))
	wav.Write(make([]byte, 480000))
	media := filepath.Join(dir, "test.wav")
	os.WriteFile(media, wav.Bytes(), 0600)
	probe := filepath.Join(dir, "probe.lua")
	source := `local mp=require("mp")
local utils=require("mp.utils")
local o={events="",status="",marker=""};require("mp.options").read_options(o,"probe")
local revision=0
local sequence=0
local occurrence=""
local failed=false
local function check(value,label)
 if mp.get_property_native("pause")~=value then
  failed=true;local f=io.open(o.marker,"a");f:write(label .. " failed\n");f:close()
 end
end
local function publish(state,held,stale)
 local f=io.open(o.events,"r")
 if f then for line in f:lines() do local e=utils.parse_json(line)
  if e and (e.event=="file-loaded" or e.event=="playback-repeat") then occurrence=e.eventId end
  if e and e.event=="intent" then sequence=e.intentSequence end
 end f:close() end
 revision=revision+1
 f=io.open(o.status,"w");f:write(utils.format_json({sessionId="test",occurrenceId=occurrence,intentSequence=stale and sequence-1 or sequence,revision=revision,heartbeat=revision,state=state,hold=held,message=state}));f:close()
end
mp.add_timeout(0.5,function() publish("playing",false) end)
mp.add_timeout(1.0,function() check(false,"initial release");publish("recovering",true) end)
mp.add_timeout(1.5,function() check(true,"recovery hold");mp.commandv("script-message","playlistmaker-pause") end)
mp.add_timeout(2.0,function() publish("playing",false) end)
mp.add_timeout(2.5,function() check(true,"user pause during recovery");mp.commandv("script-message","playlistmaker-retry") end)
mp.add_timeout(3.0,function() publish("blocked",true) end)
mp.add_timeout(3.5,function() mp.set_property_native("pause",false) end)
mp.add_timeout(4.0,function() check(true,"first Play bypass");mp.set_property_native("pause",false) end)
mp.add_timeout(4.5,function() check(true,"second Play bypass");publish("playing",false,true) end)
mp.add_timeout(5.0,function() check(true,"stale acknowledgement");publish("playing",false) end)
mp.add_timeout(5.5,function() check(false,"healthy release") end)
mp.add_timeout(21.0,function() check(true,"heartbeat loss");mp.commandv("script-message","playlistmaker-untracked") end)
mp.add_timeout(21.5,function()
 check(false,"explicit untracked")
 local f=io.open(o.marker,"a");if not failed then f:write("ok") end;f:close();mp.commandv("quit")
end)`
	os.WriteFile(probe, []byte(source), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, mpv, "--no-config", "--ao=null", "--vo=null", "--script="+script, "--script="+probe,
		"--script-opt=playlistmaker_history-manifest_path="+manifest, "--script-opt=playlistmaker_history-event_path="+events,
		"--script-opt=probe-events="+events, "--script-opt=probe-status="+status, "--script-opt=probe-marker="+marker, media).CombinedOutput()
	result, _ := os.ReadFile(marker)
	if err != nil || string(result) != "ok" {
		t.Fatalf("mpv control: %v, %s\n%s", err, result, output)
	}
	contents, _ := os.ReadFile(events)
	var intents []bool
	for _, line := range bytes.Split(contents, []byte("\n")) {
		var e struct {
			Event  string `json:"event"`
			Paused bool   `json:"paused"`
		}
		if json.Unmarshal(line, &e) == nil && e.Event == "intent" {
			intents = append(intents, e.Paused)
		}
	}
	if len(intents) < 2 || intents[0] || !intents[1] {
		t.Fatalf("system hold became user pause: %v\n%s", intents, contents)
	}
	t.Run("actual-helper", func(t *testing.T) {
		path, m, err := tracksession.Create(t.TempDir(), []tracksession.Entry{{VideoPath: media, Track: tracking.Track{SpotifyURI: "spotify:track:synthetic"}}}, false, false, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		m.MPVProcessID = 123
		if err = tracksession.WriteManifest(path, m); err != nil {
			t.Fatal(err)
		}
		player := &mpvTestSpotify{}
		runner := tracksession.Runner{Runtime: &tracksession.Runtime{Spotify: player}, Poll: 10 * time.Millisecond, IsAlive: func(int) bool { return true }}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- runner.Run(ctx, path) }()
		probe := filepath.Join(filepath.Dir(path), "helper-probe.lua")
		marker := filepath.Join(filepath.Dir(path), "result")
		source := `local mp=require("mp")
local o={marker=""};require("mp.options").read_options(o,"probe")
local failed=false
local function check(paused,label)
 if mp.get_property_native("pause")~=paused then
  failed=true;local f=io.open(o.marker,"a");f:write(label .. " failed\n");f:close()
 end
end
mp.add_timeout(0.8,function() check(false,"start");mp.commandv("script-message","playlistmaker-pause") end)
mp.add_timeout(1.3,function() check(true,"manual pause") end)
mp.add_timeout(1.6,function() mp.commandv("script-message","playlistmaker-retry") end)
mp.add_timeout(2.3,function() check(false,"manual resume") end)
mp.add_timeout(3.7,function() check(true,"failure hold");mp.commandv("script-message","playlistmaker-retry") end)
mp.add_timeout(4.5,function() check(false,"recovered");local f=io.open(o.marker,"a");if not failed then f:write("ok") end;f:close();mp.commandv("quit") end)`
		if err = os.WriteFile(probe, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		output, err := exec.CommandContext(ctx, mpv, "--no-config", "--ao=null", "--vo=null", "--script="+script, "--script="+probe,
			"--script-opt=playlistmaker_history-manifest_path="+path, "--script-opt=playlistmaker_history-event_path="+m.EventPath, "--script-opt=probe-marker="+marker, media).CombinedOutput()
		result, _ := os.ReadFile(marker)
		if err != nil || string(result) != "ok" {
			cancel()
			<-done
			t.Fatalf("actual helper: %v %s\n%s", err, result, output)
		}
		if err = <-done; err != nil {
			t.Fatal(err)
		}
		if len(player.Started) != 1 || !player.recovered {
			t.Fatal("helper restarted occurrence or failed to retry")
		}
	})
}

type mpvTestSpotify struct {
	tracking.Fake
	started   time.Time
	phase     string
	recovered bool
}

func (p *mpvTestSpotify) Preflight(context.Context, string) error { return nil }
func (p *mpvTestSpotify) Start(ctx context.Context, track tracking.Track) error {
	p.started = time.Now()
	p.phase = "playing"
	return p.Fake.Start(ctx, track)
}
func (p *mpvTestSpotify) Finished(context.Context) (bool, error) {
	if time.Since(p.started) >= 3*time.Second && !p.recovered {
		p.phase = "blocked"
		return false, &tracking.PlaybackFailure{Message: "Synthetic Spotify interruption"}
	}
	return false, nil
}
func (p *mpvTestSpotify) Intent(bool, *int)                {}
func (p *mpvTestSpotify) TrackingStatus() (string, string) { return p.phase, p.phase }
func (p *mpvTestSpotify) Retry()                           { p.recovered = true; p.phase = "playing" }
