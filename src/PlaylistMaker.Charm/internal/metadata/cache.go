// Package metadata maintains the small FLAC metadata cache used by Charm.
package metadata

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"playlistmaker/charm/internal/pathid"
)

type Entry struct {
	FilePath    string `json:"filePath"`
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	TrackNumber int    `json:"trackNumber"`
	LastRead    string `json:"lastRead,omitempty"`
}

type Reader interface {
	Read(context.Context, string) (Entry, error)
}

type FLACReader struct{}

func (FLACReader) Read(ctx context.Context, path string) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Entry{}, err
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != "fLaC" {
		return Entry{}, fmt.Errorf("not a FLAC file")
	}
	values := map[string]string{}
	for {
		block := make([]byte, 4)
		if _, err := io.ReadFull(file, block); err != nil {
			return Entry{}, fmt.Errorf("read FLAC metadata: %w", err)
		}
		last, kind := block[0]&0x80 != 0, block[0]&0x7f
		size := int(block[1])<<16 | int(block[2])<<8 | int(block[3])
		data := make([]byte, size)
		if _, err := io.ReadFull(file, data); err != nil {
			return Entry{}, fmt.Errorf("read FLAC metadata: %w", err)
		}
		if kind == 4 {
			values = vorbisComments(data)
		}
		if last {
			break
		}
	}
	entry := Entry{FilePath: pathid.Normalize(path), Artist: values["ARTIST"], Title: values["TITLE"], Date: values["DATE"], TrackNumber: trackNumber(values["TRACKNUMBER"]), LastRead: time.Now().UTC().Format(time.RFC3339)}
	if entry.Artist == "" || entry.Title == "" {
		return Entry{}, fmt.Errorf("FLAC tags require ARTIST and TITLE")
	}
	return entry, nil
}

func vorbisComments(data []byte) map[string]string {
	values := map[string]string{}
	read := func(offset *int) ([]byte, bool) {
		if *offset+4 > len(data) {
			return nil, false
		}
		length := int(binary.LittleEndian.Uint32(data[*offset:]))
		*offset += 4
		if length < 0 || *offset+length > len(data) {
			return nil, false
		}
		value := data[*offset : *offset+length]
		*offset += length
		return value, true
	}
	offset := 0
	if _, ok := read(&offset); !ok || offset+4 > len(data) {
		return values
	}
	count := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	for range count {
		value, ok := read(&offset)
		if !ok {
			break
		}
		key, text, found := strings.Cut(string(value), "=")
		if found && values[strings.ToUpper(key)] == "" {
			values[strings.ToUpper(key)] = text
		}
	}
	return values
}

func trackNumber(value string) int {
	part, _, _ := strings.Cut(strings.TrimSpace(value), "/")
	number, err := strconv.Atoi(part)
	if err != nil || number < 1 {
		return 1
	}
	return number
}

func Ensure(ctx context.Context, cachePath string, audioPaths []string, reader Reader) (map[string]Entry, bool, error) {
	entries, err := ReadCache(cachePath)
	if err != nil {
		return nil, false, err
	}
	unique := make(map[string]string)
	for _, path := range audioPaths {
		if path != "" {
			unique[pathid.ComparisonKey(path)] = pathid.Normalize(path)
		}
	}
	missing := make([]string, 0)
	for key, path := range unique {
		if _, ok := entries[key]; !ok {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	changed := false
	failures := make([]string, 0)
	for _, path := range missing {
		if err := ctx.Err(); err != nil {
			return nil, changed, err
		}
		entry, readErr := reader.Read(ctx, path)
		if readErr != nil {
			failures = append(failures, filepath.Base(path))
			continue
		}
		entry.FilePath = pathid.Normalize(path)
		entry.TrackNumber = trackNumber(strconv.Itoa(entry.TrackNumber))
		if entry.LastRead == "" {
			entry.LastRead = time.Now().UTC().Format(time.RFC3339)
		}
		entries[pathid.ComparisonKey(entry.FilePath)] = entry
		changed = true
	}
	if changed {
		if err := WriteCache(cachePath, entries); err != nil {
			return nil, false, err
		}
	}
	if len(failures) > 0 {
		return entries, changed, fmt.Errorf("could not read %d mapped FLAC file(s) (for example: %s)", len(failures), strings.Join(failures[:min(3, len(failures))], ", "))
	}
	return entries, changed, nil
}

func ReadCache(path string) (map[string]Entry, error) {
	entries := make(map[string]Entry)
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read FLAC cache: %w", err)
	}
	var decoded []Entry
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(contents), "\ufeff")), &decoded); err != nil {
		return nil, fmt.Errorf("parse FLAC cache: %w", err)
	}
	for index, entry := range decoded {
		if strings.TrimSpace(entry.FilePath) == "" || strings.TrimSpace(entry.Artist) == "" || strings.TrimSpace(entry.Title) == "" {
			return nil, fmt.Errorf("FLAC cache contains invalid entry %d", index+1)
		}
		entry.FilePath = pathid.Normalize(entry.FilePath)
		entry.TrackNumber = trackNumber(strconv.Itoa(entry.TrackNumber))
		entries[pathid.ComparisonKey(entry.FilePath)] = entry
	}
	return entries, nil
}

func WriteCache(path string, entries map[string]Entry) error {
	ordered := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		ordered = append(ordered, entry)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return pathid.ComparisonKey(ordered[i].FilePath) < pathid.ComparisonKey(ordered[j].FilePath)
	})
	contents, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".flac-cache-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err = temporary.Write(contents); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
