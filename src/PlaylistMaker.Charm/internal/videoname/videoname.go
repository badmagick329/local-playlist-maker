// Package videoname parses the naming convention used by mapped video files.
package videoname

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type Parsed struct {
	Date    string
	Artist  string
	Title   string
	Variant string
}

var (
	datePrefix   = regexp.MustCompile(`^(\d{6})\s+`)
	textVariant  = regexp.MustCompile(`(?i)\s+(Performance(?:\s+[1-9]\d*)?|Choreography|Relay|Be Original)$`)
	parenVariant = regexp.MustCompile(`\s+\(([^()]*)\)$`)
)

func Parse(value string) Parsed {
	name := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	result := Parsed{}
	if match := datePrefix.FindStringSubmatch(name); match != nil {
		result.Date = match[1]
		name = strings.TrimSpace(strings.TrimPrefix(name, match[0]))
	}
	artist, title, found := strings.Cut(name, " - ")
	if !found {
		return result
	}
	result.Artist = strings.TrimSpace(artist)
	result.Title, result.Variant = splitVariant(strings.TrimSpace(title))
	return result
}

func splitVariant(title string) (string, string) {
	if match := textVariant.FindStringSubmatch(title); match != nil {
		return strings.TrimSpace(strings.TrimSuffix(title, match[0])), strings.TrimSpace(match[1])
	}
	match := parenVariant.FindStringSubmatch(title)
	if match == nil || !recognizedParenthesized(match[1]) {
		return title, ""
	}
	return strings.TrimSpace(strings.TrimSuffix(title, match[0])), strings.TrimSpace(match[1])
}

func recognizedParenthesized(value string) bool {
	value = Normalize(value)
	switch {
	case value == "live" || strings.Contains(value, "hd live") || strings.Contains(value, "live uhd"):
		return true
	case strings.Contains(value, "band live"), strings.Contains(value, "fancam"), strings.Contains(value, "concert"), strings.Contains(value, "live audio"):
		return true
	case value == "remix" || value == "areia remix":
		return true
	default:
		return false
	}
}

// Normalize makes matching case-insensitive while retaining Unicode letters and numbers.
func Normalize(value string) string {
	value = strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, value))
	return strings.Join(strings.Fields(value), " ")
}
