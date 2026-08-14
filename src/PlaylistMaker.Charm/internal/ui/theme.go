package ui

import "charm.land/lipgloss/v2"

type theme struct {
	accent       lipgloss.Style
	title        lipgloss.Style
	muted        lipgloss.Style
	selected     lipgloss.Style
	queued       lipgloss.Style
	variant      lipgloss.Style
	bar          lipgloss.Style
	overlay      lipgloss.Style
	overlayTitle lipgloss.Style
	warning      lipgloss.Style
}

func newTheme() theme {
	return theme{
		accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("#55D6BE")).Bold(true),
		title:        lipgloss.NewStyle().Foreground(lipgloss.Color("#E8EAF0")),
		muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("#74798A")),
		selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#10131F")).Background(lipgloss.Color("#8BD5FF")),
		queued:       lipgloss.NewStyle().Foreground(lipgloss.Color("#F5C2E7")).Bold(true),
		variant:      lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")),
		bar:          lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).Background(lipgloss.Color("#25283A")),
		overlay:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#55D6BE")).Padding(1, 2).Background(lipgloss.Color("#202231")),
		overlayTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#8BD5FF")).Bold(true),
		warning:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB387")),
	}
}
