package ui

type shortcut struct{ section, key, description string }

var shortcuts = []shortcut{
	{"Navigation", "j/k, arrows", "move"}, {"Navigation", "Ctrl+U/D, PgUp/Dn", "page"}, {"Navigation", "gg/G", "first/last"}, {"Navigation", "h/l, left/right, Enter", "collapse, expand, or queue a video"},
	{"Views", "/", "search"}, {"Views", "c / s / f / p", "categories / sort / filters / playback options"}, {"Views", "?", "shortcut help"},
	{"Views", "d", "selected media details"},
	{"Views", "R", "refresh history"},
	{"Queue", "Space", "toggle current and move down"}, {"Queue", "a / A", "all current-track videos / one per filtered track"}, {"Queue", "q", "queue overlay"}, {"Queue overlay", "Shift+J/K", "reorder"}, {"Queue overlay", "Delete / Backspace", "remove"},
	{"Options", "Space", "toggle boolean or cycle version choice"}, {"Options", "digits / Backspace", "edit repeat or maximum"}, {"Options", "h/l, left/right", "adjust numeric value or choice"}, {"Options", "r / Enter / Esc", "reset / save / cancel"},
	{"Filters", "Track release / Video date", "independent FLAC release and video-date filters"}, {"Filters", "type date or range", "YYYY, YYYY-MM, YYYY-MM-DD, or START..END"}, {"Filters", "Ctrl+U / r", "clear field / reset all"}, {"Filters", "Enter / f / Esc", "apply / cancel"},
	{"Help", "j/k, Ctrl+U/D, PgUp/Dn", "scroll or page"}, {"Help", "gg/G", "first/last"}, {"Help", "? / Esc", "close"},
	{"Playback", "o", "play queue or highlighted media"}, {"Modes", "Esc", "cancel/close"}, {"Modes", "Ctrl+Q", "quit"},
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

func footerHint(current mode, width int) string {
	if current == modeNavigate {
		switch {
		case width >= 115:
			return "j/k move  h/l fold  space queue  o play  p options  / search  c categories  s sort  f filters  ? help"
		case width >= 75:
			return "o play  / search  c categories  s sort  f filters  ? help"
		case width >= 50:
			return "o play  / search  c cats  s sort  f filter  ? help"
		default:
			return "o play  / search  ? help"
		}
	}

	var hint string
	switch current {
	case modeSearch:
		hint = "type search  •  space inserts space  •  ctrl+j/k or arrows move  •  ctrl+u clear  •  enter, /, esc nav"
	case modeCategories:
		hint = "j/k move  •  space/enter toggle  •  c/esc close"
	case modeSort:
		hint = "j/k move  •  enter/space apply  •  s/esc close"
	case modeQueue:
		hint = "j/k move  •  shift+j/k reorder  •  delete/backspace/space remove  •  q/esc close"
	case modePlaybackOptions:
		hint = "j/k move  •  space toggle  •  h/l adjust  •  digits edit  •  r reset  •  enter save  •  p/esc cancel"
	case modeFilters:
		hint = "j/k move  •  type date/range  •  ctrl+u clear  •  r reset  •  enter apply  •  f/esc cancel"
	case modeHelp:
		hint = "j/k scroll  •  ctrl+u/d or pgup/dn page  •  gg/G first/last  •  ?/esc close"
	case modeDetails:
		hint = "j/k scroll  •  ctrl+u/d or pgup/dn page  •  gg/G first/last  •  d/esc close"
	}
	return truncate(hint, width)
}
