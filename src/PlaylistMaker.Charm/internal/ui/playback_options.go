package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleOptionsKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "space":
		if m.overlayCursor == 0 {
			m.draftOptions.Shuffle = !m.draftOptions.Shuffle
		}
		if m.overlayCursor == 1 {
			m.draftOptions.OneVideoPerTrack = !m.draftOptions.OneVideoPerTrack
		}
		if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(1)
		}
		return m
	case "h", "left":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = max(1, m.draftOptions.RepeatEach-1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems = max(0, m.draftOptions.MaximumItems-1)
		} else if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(-1)
		}
		return m
	case "l", "right":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = min(10, m.draftOptions.RepeatEach+1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems++
		} else if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(1)
		}
		return m
	case "backspace":
		if m.overlayCursor >= 2 && m.overlayCursor <= 3 {
			if m.optionEditField != m.overlayCursor {
				m.optionEditField = m.overlayCursor
				m.optionEdit = m.optionValue()
			}
			if m.optionEdit != "" {
				m.optionEdit = m.optionEdit[:len(m.optionEdit)-1]
			}
			m.applyOptionEdit()
		}
		return m
	}
	if key.Text >= "0" && key.Text <= "9" {
		if m.overlayCursor >= 2 && m.overlayCursor <= 3 {
			if m.optionEditField != m.overlayCursor {
				m.optionEditField, m.optionEdit = m.overlayCursor, ""
			}
			m.optionEdit += key.Text
			m.applyOptionEdit()
		}
	}
	return m
}

func (m *Model) clearOptionEdit() {
	m.optionEdit, m.optionError, m.optionEditField = "", "", -1
}

func (m Model) optionValue() string {
	if m.overlayCursor == 2 {
		return strconv.Itoa(m.draftOptions.RepeatEach)
	}
	return strconv.Itoa(m.draftOptions.MaximumItems)
}

func (m *Model) applyOptionEdit() bool {
	if m.optionEdit == "" {
		m.optionError = "A number is required"
		return false
	}
	value, err := strconv.Atoi(m.optionEdit)
	if err != nil {
		m.optionError = "Number is too large"
		return false
	}
	if m.optionEditField == 2 {
		if value < 1 || value > 10 {
			m.optionError = "Repeat must be 1 through 10"
			return false
		}
		m.draftOptions.RepeatEach = value
	} else if m.optionEditField == 3 {
		if value < 0 {
			m.optionError = "Maximum must be zero or greater"
			return false
		}
		m.draftOptions.MaximumItems = value
	}
	m.optionError = ""
	return true
}

func (m *Model) commitOptionEdit() bool {
	if m.optionEditField < 0 {
		return true
	}
	if !m.applyOptionEdit() {
		return false
	}
	m.clearOptionEdit()
	return true
}

func (m Model) playbackOptionsOverlay(height int) (string, []string) {
	var lines []string
	title := "Playback"
	lines = m.playbackPanelLines(height)
	return title, lines
}

func (m Model) optionDisplay(field int) string {
	if m.optionEditField == field {
		if m.optionEdit == "" {
			return "_"
		}
		return m.optionEdit + "_"
	}
	if field == 2 {
		return strconv.Itoa(m.draftOptions.RepeatEach)
	}
	return strconv.Itoa(m.draftOptions.MaximumItems)
}
