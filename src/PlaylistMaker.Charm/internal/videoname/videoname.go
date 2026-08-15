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
	textVariant  = regexp.MustCompile(`(?i)\s+(Performance(?:\s+[1-9]\d*)?|Choreography|Relay|Be Original)$`)
	parenVariant = regexp.MustCompile(`\s+\(([^()]*)\)$`)
)

func Parse(value string) Parsed {
	name := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	result := Parsed{}
	if date, remainder, ok := datePrefix(name); ok {
		result.Date = date
		name = remainder
	}
	artist, title, found := strings.Cut(name, " - ")
	if !found {
		return result
	}
	result.Artist = strings.TrimSpace(artist)
	result.Title, result.Variant = splitVariant(strings.TrimSpace(title))
	return result
}

func datePrefix(name string) (string, string, bool) {
	for _, length := range []int{8, 6} {
		if len(name) <= length || !digits(name[:length]) {
			continue
		}
		remainder := strings.TrimSpace(name[length:])
		if len(remainder) < len(name[length:]) {
			return name[:length], remainder, true
		}
	}
	return "", name, false
}

func digits(value string) bool {
	for _, value := range value {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
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
