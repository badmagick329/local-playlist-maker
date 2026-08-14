package pathid

import "testing"

func TestComparisonKeyIsCaseInsensitiveAndKeepsLibraryCompatibilityReplacement(t *testing.T) {
	left := ComparisonKey(`C:\Music\Library\Track.flac`)
	right := ComparisonKey(`c:\music/Library/track.flac`)
	if left != right {
		t.Fatalf("comparison keys differ: %q != %q", left, right)
	}
}
