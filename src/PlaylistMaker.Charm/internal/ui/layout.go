package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func onOff(value bool) string {
	if value {
		return "On"
	}
	return "Off"
}

func cursorMark(cursor, index int) string {
	if cursor == index {
		return "›"
	}
	return " "
}

func emptyAny(value string) string {
	if value == "" {
		return "Any"
	}
	return value
}

func availability(value bool) string {
	if value {
		return "available"
	}
	return "missing"
}

func joinAligned(left, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width-8 {
		return truncate(left, width)
	}
	availableLeft := max(width-rightWidth-2, 1)
	if leftWidth > availableLeft {
		left = truncateANSI(left, availableLeft)
		leftWidth = lipgloss.Width(left)
	}
	return left + strings.Repeat(" ", max(width-leftWidth-rightWidth, 1)) + right
}

func joinAlignedWithSuffix(prefix, suffix, right string, width int) string {
	return joinAlignedWithSuffixFillInternal(prefix, suffix, right, width, nil)
}

func joinAlignedWithSuffixFill(prefix, suffix, right string, width int, fill lipgloss.Style) string {
	return joinAlignedWithSuffixFillInternal(prefix, suffix, right, width, &fill)
}

func joinAlignedWithSuffixFillInternal(prefix, suffix, right string, width int, fill *lipgloss.Style) string {
	rightWidth := lipgloss.Width(right)
	availableLeft := max(width-rightWidth-2, 1)
	suffixWidth := lipgloss.Width(suffix)
	if lipgloss.Width(prefix)+suffixWidth > availableLeft {
		prefix = truncateANSI(prefix, max(availableLeft-suffixWidth, 1))
	}
	left := prefix + suffix
	leftWidth := lipgloss.Width(left)
	if rightWidth >= width-8 {
		return truncate(left, width)
	}
	gap := strings.Repeat(" ", max(width-leftWidth-rightWidth, 1))
	if fill != nil {
		gap = fill.Render(gap)
	}
	return left + gap + right
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func truncateANSI(value string, width int) string {
	return ansi.Truncate(value, width, "…")
}

func stripStyles(value string) string {
	return ansi.Strip(value)
}

func padLines(value string, height int) string {
	lineCount := strings.Count(value, "\n") + 1
	if lineCount >= height {
		return value
	}
	return value + strings.Repeat("\n", height-lineCount)
}

func placeOverlay(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	overlayLines := strings.Split(overlay, "\n")
	overlayWidth := 0
	for _, line := range overlayLines {
		overlayWidth = max(overlayWidth, lipgloss.Width(line))
	}
	x := max((width-overlayWidth)/2, 0)
	y := max((height-len(overlayLines))/2, 0)
	for index, overlayLine := range overlayLines {
		row := y + index
		if row >= len(baseLines) {
			break
		}
		background := ansi.Truncate(stripStyles(baseLines[row]), width, "")
		background += strings.Repeat(" ", max(width-ansi.StringWidth(background), 0))
		left := ansi.Cut(background, 0, x)
		left += strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
		right := ansi.Cut(background, x+overlayWidth, width)
		baseLines[row] = left + overlayLine + right
	}
	return strings.Join(baseLines[:height], "\n")
}
