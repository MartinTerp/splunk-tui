package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines keyboard shortcuts across all views.
type KeyMap struct {
	SearchFocus key.Binding
	TimePicker  key.Binding
	RunSearch   key.Binding
	CancelSearch key.Binding
	NextPage    key.Binding
	PrevPage    key.Binding
	ToggleChart key.Binding
	OpenBrowser key.Binding
	IncludeField key.Binding
	ExcludeField key.Binding
	ToggleCol   key.Binding
	Detail      key.Binding
	NextPanel   key.Binding
	PrevPanel   key.Binding
	Help        key.Binding
	Quit        key.Binding
	Escape      key.Binding
}

// DefaultKeyMap returns the default keyboard shortcuts.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		SearchFocus: key.NewBinding(
			key.WithKeys("/", "ctrl+f"),
			key.WithHelp("/, ctrl+f", "search"),
		),
		TimePicker: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "time range"),
		),
		RunSearch: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "run query"),
		),
		CancelSearch: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "cancel job"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("n", "pgdown"),
			key.WithHelp("n, pgdn", "next page"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("p", "pgup"),
			key.WithHelp("p, pgup", "prev page"),
		),
		ToggleChart: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "toggle chart"),
		),
		OpenBrowser: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "instances"),
		),
		IncludeField: key.NewBinding(
			key.WithKeys("+"),
			key.WithHelp("+", "include field"),
		),
		ExcludeField: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "exclude field"),
		),
		ToggleCol: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "toggle column"),
		),
		Detail: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "view detail"),
		),
		NextPanel: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		PrevPanel: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q, ctrl+c", "quit"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/cancel"),
		),
	}
}
