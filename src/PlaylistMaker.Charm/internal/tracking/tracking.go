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
	Start(context.Context, string, []string) error
	Run(context.Context, string, []string) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Start(ctx context.Context, program string, arguments []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	command := exec.Command(program, arguments...)
	command.Stdin, command.Stdout, command.Stderr = nil, io.Discard, io.Discard
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

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
	return p.start(ctx, p.StartCommand, track)
}

func (p Local) Stop(ctx context.Context) error  { return p.run(ctx, p.StopCommand, Track{}) }
func (p Local) Close(ctx context.Context) error { return p.Stop(ctx) }

func (p Local) run(ctx context.Context, command []string, track Track) error {
	program, arguments, runner, err := p.command(command, track)
	if err != nil {
		return err
	}
	if err := runner.Run(ctx, program, arguments); err != nil {
		return fmt.Errorf("run local tracking command: %w", err)
	}
	return nil
}

func (p Local) start(ctx context.Context, command []string, track Track) error {
	program, arguments, runner, err := p.command(command, track)
	if err != nil {
		return err
	}
	if err := runner.Start(ctx, program, arguments); err != nil {
		return fmt.Errorf("start local tracking command: %w", err)
	}
	return nil
}

func (p Local) command(command []string, track Track) (string, []string, CommandRunner, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return "", nil, nil, fmt.Errorf("local tracking command is not configured")
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
	return command[0], arguments, runner, nil
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
