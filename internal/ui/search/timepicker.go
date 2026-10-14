package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/ui/theme"
)

// TimeRange represents a Splunk time window.
type TimeRange struct {
	Label    string
	Earliest string
	Latest   string
}

// TimePreset defines a standard earliest start preset.
type TimePreset struct {
	Label    string
	Earliest string
	Latest   string
}

// LatestPreset defines a standard latest end preset.
type LatestPreset struct {
	Label  string
	Latest string
}

// DefaultEarliestPresets provides standard relative time start options.
var DefaultEarliestPresets = []TimePreset{
	{Label: "Last 15 minutes", Earliest: "-15m", Latest: "now"},
	{Label: "Last 60 minutes", Earliest: "-60m", Latest: "now"},
	{Label: "Last 4 hours", Earliest: "-4h", Latest: "now"},
	{Label: "Last 24 hours", Earliest: "-24h", Latest: "now"},
	{Label: "Last 7 days", Earliest: "-7d", Latest: "now"},
	{Label: "Last 30 days", Earliest: "-30d", Latest: "now"},
	{Label: "Today", Earliest: "@d", Latest: "now"},
	{Label: "Yesterday", Earliest: "-1d@d", Latest: "@d"},
	{Label: "Previous week", Earliest: "-7d@w0", Latest: "@w0"},
	{Label: "Previous month", Earliest: "-1mon@mon", Latest: "@mon"},
	{Label: "All time", Earliest: "0", Latest: ""},
}

// DefaultLatestPresets provides standard relative and boundary end options.
var DefaultLatestPresets = []LatestPreset{
	{Label: "Now (current time)", Latest: "now"},
	{Label: "Beginning of today (@d)", Latest: "@d"},
	{Label: "Beginning of hour (@h)", Latest: "@h"},
	{Label: "15 minutes ago (-15m)", Latest: "-15m"},
	{Label: "1 hour ago (-1h)", Latest: "-1h"},
	{Label: "4 hours ago (-4h)", Latest: "-4h"},
	{Label: "24 hours ago (-24h)", Latest: "-24h"},
	{Label: "Yesterday start of day (-1d@d)", Latest: "-1d@d"},
	{Label: "Start of week (@w0)", Latest: "@w0"},
	{Label: "Start of month (@mon)", Latest: "@mon"},
}

// DefaultTimePresets provides backward compatibility for existing presets.
var DefaultTimePresets = []TimeRange{
	{Label: "Last 15 minutes", Earliest: "-15m", Latest: "now"},
	{Label: "Last 60 minutes", Earliest: "-60m", Latest: "now"},
	{Label: "Last 4 hours", Earliest: "-4h", Latest: "now"},
	{Label: "Last 24 hours", Earliest: "-24h", Latest: "now"},
	{Label: "Last 7 days", Earliest: "-7d", Latest: "now"},
	{Label: "Last 30 days", Earliest: "-30d", Latest: "now"},
	{Label: "Today", Earliest: "@d", Latest: "now"},
	{Label: "Yesterday", Earliest: "-1d@d", Latest: "@d"},
	{Label: "Previous week", Earliest: "-7d@w0", Latest: "@w0"},
	{Label: "Previous month", Earliest: "-1mon@mon", Latest: "@mon"},
	{Label: "All time", Earliest: "0", Latest: ""},
}

// PickerTarget specifies whether the open picker targets the Earliest or Latest boundary.
type PickerTarget int

const (
	TargetEarliest PickerTarget = iota
	TargetLatest
)

// TimePickerModel manages both Earliest (From) and Latest (To) time boundaries.
type TimePickerModel struct {
	theme         theme.Theme
	earliest      string
	latest        string
	target        PickerTarget
	isOpen        bool
	selectedIndex int // -1 means custom textinput is focused; >= 0 means a preset is highlighted
	customInput   textinput.Model
}

// NewTimePicker creates a time picker model with default earliest and latest bounds.
func NewTimePicker(th theme.Theme, defaultEarliest string) TimePickerModel {
	if strings.TrimSpace(defaultEarliest) == "" {
		defaultEarliest = "-24h"
	}

	ci := textinput.New()
	ci.Placeholder = "-24h, @d, now, 2026-10-12 00:00:00"
	ci.Prompt = "▶ "
	ci.PromptStyle = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	ci.Width = 32

	return TimePickerModel{
		theme:         th,
		earliest:      defaultEarliest,
		latest:        "now",
		selectedIndex: -1,
		customInput:   ci,
	}
}

// Earliest returns the active earliest time boundary.
func (m TimePickerModel) Earliest() string {
	if m.earliest == "" {
		return "-24h"
	}
	return m.earliest
}

// Latest returns the active latest time boundary.
func (m TimePickerModel) Latest() string {
	if m.latest == "" {
		return "now"
	}
	return m.latest
}

// SetEarliest updates the earliest time boundary.
func (m *TimePickerModel) SetEarliest(e string) {
	m.earliest = strings.TrimSpace(e)
}

// SetLatest updates the latest time boundary.
func (m *TimePickerModel) SetLatest(l string) {
	m.latest = strings.TrimSpace(l)
}

// SetTimeRange updates both earliest and latest boundaries.
func (m *TimePickerModel) SetTimeRange(e, l string) {
	m.earliest = strings.TrimSpace(e)
	m.latest = strings.TrimSpace(l)
}

// Selected returns the currently active TimeRange.
func (m TimePickerModel) Selected() TimeRange {
	return TimeRange{
		Earliest: m.Earliest(),
		Latest:   m.Latest(),
		Label:    fmt.Sprintf("%s to %s", m.Earliest(), m.Latest()),
	}
}

// Label returns the short label representing the current time window.
func (m TimePickerModel) Label() string {
	return fmt.Sprintf("%s to %s", m.Earliest(), m.Latest())
}

// OpenEarliest opens the dropdown for configuring the Earliest (From) boundary.
func (m *TimePickerModel) OpenEarliest() {
	m.target = TargetEarliest
	m.isOpen = true
	m.selectedIndex = -1
	m.customInput.SetValue(m.Earliest())
	m.customInput.Focus()
}

// OpenLatest opens the dropdown for configuring the Latest (To) boundary.
func (m *TimePickerModel) OpenLatest() {
	m.target = TargetLatest
	m.isOpen = true
	m.selectedIndex = -1
	m.customInput.SetValue(m.Latest())
	m.customInput.Focus()
}

// Open opens the picker (defaults to Earliest).
func (m *TimePickerModel) Open() {
	m.OpenEarliest()
}

// Close closes the active dropdown.
func (m *TimePickerModel) Close() {
	m.isOpen = false
}

// IsOpen reports whether any time picker dropdown is active.
func (m TimePickerModel) IsOpen() bool {
	return m.isOpen
}

// Target reports whether the open dropdown is editing Earliest or Latest.
func (m TimePickerModel) Target() PickerTarget {
	return m.target
}

// ViewFromButton renders the "From: <earliest> ▾" button for the search bar.
func (m TimePickerModel) ViewFromButton(focused bool) string {
	val := m.Earliest()
	text := fmt.Sprintf("⏱ From: %s ▾", truncate(val, 14))
	if focused {
		return m.theme.ActiveButton.Render(text)
	}
	return m.theme.Button.Render(text)
}

// ViewToButton renders the "To: <latest> ▾" button for the search bar.
func (m TimePickerModel) ViewToButton(focused bool) string {
	val := m.Latest()
	text := fmt.Sprintf("To: %s ▾", truncate(val, 14))
	if focused {
		return m.theme.ActiveButton.Render(text)
	}
	return m.theme.Button.Render(text)
}

// ViewButton provides backward compatibility for single-button rendering.
func (m TimePickerModel) ViewButton(focused bool) string {
	return m.ViewFromButton(focused)
}

// Update handles keyboard navigation and selection inside the time dropdown.
func (m TimePickerModel) Update(msg tea.Msg) (TimePickerModel, tea.Cmd, bool) {
	if !m.isOpen {
		return m, nil, false
	}

	maxPresets := len(DefaultEarliestPresets)
	if m.target == TargetLatest {
		maxPresets = len(DefaultLatestPresets)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+t", "ctrl+e":
			m.isOpen = false
			return m, nil, false

		case "down", "j":
			if m.selectedIndex == -1 {
				m.selectedIndex = 0
				m.customInput.Blur()
				return m, nil, false
			} else if m.selectedIndex < maxPresets-1 {
				m.selectedIndex++
				return m, nil, false
			}

		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				return m, nil, false
			} else if m.selectedIndex == 0 {
				m.selectedIndex = -1
				m.customInput.Focus()
				return m, nil, false
			}

		case "enter":
			if m.selectedIndex == -1 {
				val := strings.TrimSpace(m.customInput.Value())
				if val != "" {
					if m.target == TargetEarliest {
						m.earliest = val
					} else {
						m.latest = val
					}
				}
				m.isOpen = false
				return m, nil, true
			}

			// Applied from presets
			if m.target == TargetEarliest {
				p := DefaultEarliestPresets[m.selectedIndex]
				m.earliest = p.Earliest
				if p.Latest != "" && p.Latest != "now" {
					m.latest = p.Latest
				}
			} else {
				p := DefaultLatestPresets[m.selectedIndex]
				m.latest = p.Latest
			}
			m.isOpen = false
			return m, nil, true
		}
	}

	if m.selectedIndex == -1 {
		var cmd tea.Cmd
		m.customInput, cmd = m.customInput.Update(msg)
		return m, cmd, false
	}

	return m, nil, false
}

// ViewDropdown renders the floating dropdown dialog for selecting or typing time expressions.
func (m TimePickerModel) ViewDropdown() string {
	if !m.isOpen {
		return ""
	}

	var rows []string
	if m.target == TargetEarliest {
		title := m.theme.Title.Render("⏱ Select Earliest Time (From)")
		sub := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Relative (-24h, @d, -7d) or absolute (YYYY-MM-DD)")
		rows = append(rows, title, sub, "")

		inputLabel := "Custom / Enter expression:"
		if m.selectedIndex == -1 {
			inputLabel = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render("▶ Custom / Enter expression (active):")
		}
		rows = append(rows, inputLabel, m.customInput.View(), "")

		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render("Presets:"))
		for i, p := range DefaultEarliestPresets {
			prefix := "  "
			style := m.theme.TableRow
			if i == m.selectedIndex {
				prefix = "▶ "
				style = m.theme.TableSelected
			}
			rows = append(rows, style.Render(fmt.Sprintf("%s%-18s (%s)", prefix, p.Label, p.Earliest)))
		}
	} else {
		title := m.theme.Title.Render("⏱ Select Latest Time (To)")
		sub := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Relative (now, @d, -1h) or absolute (YYYY-MM-DD)")
		rows = append(rows, title, sub, "")

		inputLabel := "Custom / Enter expression:"
		if m.selectedIndex == -1 {
			inputLabel = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render("▶ Custom / Enter expression (active):")
		}
		rows = append(rows, inputLabel, m.customInput.View(), "")

		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render("Presets:"))
		for i, p := range DefaultLatestPresets {
			prefix := "  "
			style := m.theme.TableRow
			if i == m.selectedIndex {
				prefix = "▶ "
				style = m.theme.TableSelected
			}
			rows = append(rows, style.Render(fmt.Sprintf("%s%-24s (%s)", prefix, p.Label, p.Latest)))
		}
	}

	rows = append(rows, "", lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("enter: select • ↑/↓: navigate • esc: close"))
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return m.theme.Card.Width(44).Render(content)
}
