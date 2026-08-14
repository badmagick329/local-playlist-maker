package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeReader struct {
	entries map[string]Entry
	reads   []string
	err     error
}

func (f *fakeReader) Read(_ context.Context, path string) (Entry, error) {
	f.reads = append(f.reads, path)
	if f.err != nil {
		return Entry{}, f.err
	}
	entry := f.entries[path]
	if entry.Artist == "" {
		entry = Entry{Artist: "나연", Title: "Pop!", Date: "2024-05", TrackNumber: 3}
	}
	return entry, nil
}

func TestEnsureBuildsMissingUniqueCacheEntries(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "state", "flac_cache.json")
	reader := &fakeReader{entries: map[string]Entry{`C:\\Music\\one.flac`: {Artist: "나연", Title: "Pop!", Date: "2024-05", TrackNumber: 3}}}
	entries, changed, err := Ensure(context.Background(), cache, []string{`C:\\Music\\one.flac`, `c:/music/one.flac`}, reader)
	if err != nil || !changed || len(entries) != 1 || len(reader.reads) != 1 {
		t.Fatalf("ensure = %#v, %t, %v; reads=%d", entries, changed, err, len(reader.reads))
	}
	if _, changed, err = Ensure(context.Background(), cache, []string{`C:\\Music\\one.flac`}, reader); err != nil || changed || len(reader.reads) != 1 {
		t.Fatalf("complete cache was reread: changed=%t reads=%d err=%v", changed, len(reader.reads), err)
	}
	contents, err := os.ReadFile(cache)
	if err != nil || !strings.Contains(string(contents), "나연") || !strings.Contains(string(contents), "trackNumber") {
		t.Fatalf("cache content = %q, %v", contents, err)
	}
}

func TestEnsurePersistsSuccessfulEntriesBeforeReportingFailures(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache.json")
	reader := &fakeReader{err: errors.New("unreadable")}
	_, changed, err := Ensure(context.Background(), cache, []string{"bad.flac"}, reader)
	if err == nil || changed || !strings.Contains(err.Error(), "1 mapped") {
		t.Fatalf("failure = changed %t, err %v", changed, err)
	}
	if _, err := ReadCache(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("missing cache should be empty: %v", err)
	}
}

func TestTrackNumberAcceptsLeadingComponent(t *testing.T) {
	if got := trackNumber("3/12"); got != 3 {
		t.Fatalf("track number = %d", got)
	}
	if got := trackNumber("x"); got != 1 {
		t.Fatalf("track fallback = %d", got)
	}
}
