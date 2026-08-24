package tracking

import (
	"context"
	"testing"
)

type commandRecorder struct {
	starts int
	runs   int
	args   []string
}

func (r *commandRecorder) Start(_ context.Context, _ string, args []string) error {
	r.starts++
	r.args = args
	return nil
}
func (r *commandRecorder) Run(context.Context, string, []string) error { r.runs++; return nil }

func TestLocalLongRunningStartIsNonBlockingAndStopIsSynchronous(t *testing.T) {
	runner := &commandRecorder{}
	player := Local{StartCommand: []string{"foobar", "{audioPath}", "{artist}", "{title}"}, StopCommand: []string{"foobar", "/stop"}, Runner: runner}
	track := Track{TrackID: "track", LocalAudioPath: "song.flac", Artist: "Artist", Title: "Title"}
	if err := player.Start(context.Background(), track); err != nil {
		t.Fatal(err)
	}
	if runner.starts != 1 || runner.runs != 0 {
		t.Fatalf("start calls = %d, run calls = %d", runner.starts, runner.runs)
	}
	if len(runner.args) != 3 || runner.args[0] != "song.flac" || runner.args[1] != "Artist" || runner.args[2] != "Title" {
		t.Fatalf("start args = %v", runner.args)
	}
	if err := player.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.runs != 1 {
		t.Fatalf("stop run calls = %d", runner.runs)
	}
}
