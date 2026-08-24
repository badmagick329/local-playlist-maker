package tracking

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Track struct {
	TrackID        string `json:"trackId"`
	Artist         string `json:"artist"`
	Title          string `json:"title"`
	LocalAudioPath string `json:"localAudioPath,omitempty"`
	SpotifyURI     string `json:"spotifyUri,omitempty"`
}

type Player interface {
	Start(context.Context, Track) error
	Stop(context.Context) error
	Close(context.Context) error
}

type Noop struct{}

func (Noop) Start(context.Context, Track) error { return nil }
func (Noop) Stop(context.Context) error         { return nil }
func (Noop) Close(context.Context) error        { return nil }

type CommandRunner interface {
	Run(context.Context, string, []string) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, program string, arguments []string) error {
	command := exec.CommandContext(ctx, program, arguments...)
	command.Stdin, command.Stdout, command.Stderr = nil, io.Discard, io.Discard
	return command.Run()
}

type Local struct {
	StartCommand []string
	StopCommand  []string
	Runner       CommandRunner
}

func (p Local) Start(ctx context.Context, track Track) error {
	if track.LocalAudioPath == "" {
		return fmt.Errorf("track %q has no local audio", track.TrackID)
	}
	return p.run(ctx, p.StartCommand, track)
}

func (p Local) Stop(ctx context.Context) error  { return p.run(ctx, p.StopCommand, Track{}) }
func (p Local) Close(ctx context.Context) error { return p.Stop(ctx) }

func (p Local) run(ctx context.Context, command []string, track Track) error {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return fmt.Errorf("local tracking command is not configured")
	}
	arguments := make([]string, len(command)-1)
	for index, argument := range command[1:] {
		argument = strings.ReplaceAll(argument, "{audioPath}", track.LocalAudioPath)
		argument = strings.ReplaceAll(argument, "{artist}", track.Artist)
		arguments[index] = strings.ReplaceAll(argument, "{title}", track.Title)
	}
	runner := p.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}
	if err := runner.Run(ctx, command[0], arguments); err != nil {
		return fmt.Errorf("run local tracking command: %w", err)
	}
	return nil
}

type Fake struct {
	Started []Track
	Stops   int
	Closed  int
	Err     error
}

func (p *Fake) Start(_ context.Context, track Track) error {
	p.Started = append(p.Started, track)
	return p.Err
}
func (p *Fake) Stop(context.Context) error  { p.Stops++; return p.Err }
func (p *Fake) Close(context.Context) error { p.Closed++; return p.Err }
