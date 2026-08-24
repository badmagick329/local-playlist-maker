package mpvscript

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed playlistmaker-history.lua
var source []byte

func Ensure(dataDirectory string) (string, error) {
	path := filepath.Join(dataDirectory, "mpv-scripts", "playlistmaker-history.lua")
	if contents, err := os.ReadFile(path); err == nil && bytes.Equal(contents, source) {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".playlistmaker-history-*.tmp")
	if err != nil {
		return "", err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(source)
		if err == nil {
			err = temporary.Sync()
		}
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := replaceFile(name, path); err != nil {
		return "", err
	}
	return path, nil
}
