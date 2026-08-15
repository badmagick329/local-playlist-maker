package native

import "playlistmaker/charm/internal/mapping"

type mappingEntry struct {
	videoPath string
	audioPath string
}

func loadMappings(path string) ([]mappingEntry, error) {
	loaded, err := mapping.Read(path)
	if err != nil {
		return nil, err
	}
	entries := make([]mappingEntry, len(loaded))
	for index, entry := range loaded {
		entries[index] = mappingEntry{videoPath: entry.VideoPath, audioPath: entry.AudioPath}
	}
	return entries, nil
}
