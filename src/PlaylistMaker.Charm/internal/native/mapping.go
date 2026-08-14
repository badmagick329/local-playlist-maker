package native

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"playlistmaker/charm/internal/pathid"
)

type mappingEntry struct {
	videoPath string
	audioPath string
}

func loadMappings(paths []string) ([]mappingEntry, error) {
	entries := make([]mappingEntry, 0)
	positions := make(map[string]int)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("read mapping %q: %w", path, err)
		}
		decoded, err := decodeMapping(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("read mapping %q: %w", path, err)
		}
		for _, entry := range decoded {
			key := pathid.ComparisonKey(entry.videoPath)
			if position, exists := positions[key]; exists {
				entries[position] = entry
				continue
			}
			positions[key] = len(entries)
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func decodeMapping(reader io.Reader) ([]mappingEntry, error) {
	decoder := json.NewDecoder(reader)
	first, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("mapping must be a JSON object")
	}
	var result []mappingEntry
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("mapping key is not a string")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		audioPath, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("mapping value for %q must be a string", key)
		}
		result = append(result, mappingEntry{videoPath: pathid.Normalize(key), audioPath: pathid.Normalize(audioPath)})
	}
	last, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := last.(json.Delim); !ok || delimiter != '}' {
		return nil, fmt.Errorf("mapping object did not close")
	}
	return result, nil
}
