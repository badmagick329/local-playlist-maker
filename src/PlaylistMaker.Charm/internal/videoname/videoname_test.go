package videoname

import "testing"

func TestParseRecognizesDatesAndEstablishedVariants(t *testing.T) {
	for _, test := range []struct {
		filename, title, variant string
	}{
		{"260724 Billlie - Work (HD Live).webm", "Work", "HD Live"},
		{"260724 Billlie - Work (UHD Live).webm", "Work", "UHD Live"},
		{"260724 Billlie - Work (Live).webm", "Work", "Live"},
		{"260724 Billlie - Work (HD Live Event).webm", "Work", "HD Live Event"},
		{"Billlie - Work Performance 2.webm", "Work", "Performance 2"},
		{"Billlie - Work Performance.webm", "Work", "Performance"},
		{"Billlie - Work Choreography.webm", "Work", "Choreography"},
		{"Billlie - Work Relay.webm", "Work", "Relay"},
		{"Billlie - Work Be Original.webm", "Work", "Be Original"},
		{"Billlie - Work (Band Live).webm", "Work", "Band Live"},
		{"Billlie - Work (Jihyo Fancam).webm", "Work", "Jihyo Fancam"},
		{"Billlie - Work (Japan Tour Concert).webm", "Work", "Japan Tour Concert"},
		{"Billlie - Work (Remix).webm", "Work", "Remix"},
		{"Billlie - Work (Live Audio).webm", "Work", "Live Audio"},
	} {
		parsed := Parse(test.filename)
		if parsed.Artist != "Billlie" || parsed.Title != test.title || parsed.Variant != test.variant {
			t.Fatalf("Parse(%q) = %#v", test.filename, parsed)
		}
	}
	if parsed := Parse("260724 Billlie - WDA (Whole Different Animal).webm"); parsed.Date != "260724" || parsed.Title != "WDA (Whole Different Animal)" || parsed.Variant != "" {
		t.Fatalf("unrecognized parentheses were removed: %#v", parsed)
	}
	for _, filename := range []string{"241029 ITZY - Imaginary Friend (Areia Remix).webm", "20241029 ITZY - Imaginary Friend (Areia Remix).webm"} {
		parsed := Parse(filename)
		if parsed.Artist != "ITZY" || parsed.Title != "Imaginary Friend" || parsed.Variant != "Areia Remix" {
			t.Fatalf("Parse(%q) = %#v", filename, parsed)
		}
	}
}

func TestNormalizeCollapsesPunctuationWhitespaceAndPreservesUnicode(t *testing.T) {
	if got := Normalize("  BILLlie...  나연 — Work!  "); got != "billlie 나연 work" {
		t.Fatalf("Normalize = %q", got)
	}
}
