// Package pathid defines file-path identity for the Windows-focused media library.
package pathid

import (
	"path/filepath"
	"strings"
)

func Resolve(baseDirectory, value string) string {
	if value == "" {
		return ""
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(baseDirectory, value)
	}
	return filepath.Clean(value)
}

func Normalize(value string) string {
	return Resolve("", strings.ReplaceAll(value, "/Library/", `\Library\`))
}

// ComparisonKey is intentionally case-insensitive: playlist identities are Windows
// paths even when tests run on a case-sensitive filesystem.
func ComparisonKey(value string) string {
	return strings.ToLower(Normalize(value))
}
