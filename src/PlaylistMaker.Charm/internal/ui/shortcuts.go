package ui

type shortcut struct{ section, key, description string }

var shortcuts = []shortcut{
	{"Navigation", "j/k, arrows", "move"}, {"Navigation", "Ctrl+U/D, PgUp/Dn", "page"}, {"Navigation", "gg/G", "first/last"},
	{"Views", "/", "search"}, {"Views", "c/s/f/o", "categories, sort, filters, options"}, {"Views", "?", "help"},
	{"Queue", "Space", "toggle current"}, {"Queue", "a / A", "queue defaults / all"}, {"Queue", "q", "queue overlay"},
	{"Playback", "Ctrl+Enter", "launch queue"}, {"Modes", "Esc", "cancel/close"}, {"Modes", "Ctrl+Q", "quit"},
}

func helpLines() []string {
	lines, section := []string{}, ""
	for _, item := range shortcuts {
		if item.section != section {
			section = item.section
			lines = append(lines, "", section)
		}
		lines = append(lines, item.key+"  —  "+item.description)
	}
	return lines
}
