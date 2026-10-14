package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UnlockDoneMsg is sent when the user successfully submits the master password.
type UnlockDoneMsg struct {
	Password string
}

// UnlockModel manages master password prompt for encrypted file keyring.
type UnlockModel struct {
	theme     Theme
	input     textinput.Model
	width     int
	height    int
	errMsg    string
}

// NewUnlockModel initializes the unlock view.
func NewUnlockModel(theme Theme) UnlockModel {
	ti := textinput.New()
	ti.Placeholder = "Enter master password to unlock keyring..."
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 40

	return UnlockModel{
		theme: theme,
		input: ti,
	}
}

func (m UnlockModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m UnlockModel) Update(msg tea.Msg) (UnlockModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if val == "" {
				m.errMsg = "Password cannot be empty"
				return m, nil
			}
			return m, func() tea.Msg {
				return UnlockDoneMsg{Password: val}
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// SetError updates the displayed error message.
func (m *UnlockModel) SetError(err string) {
	m.errMsg = err
}

func (m UnlockModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}

	header := m.theme.Header.Width(innerW).Render("⚡ Splunk TUI  │  Unlock Credential Store")

	bodyCardWidth := w - 4
	if bodyCardWidth < 20 {
		bodyCardWidth = 20
	}
	bodyHeight := h - 4
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	title := m.theme.Title.Render("🔒 Unlock Splunk Credentials")
	desc := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(
		"splunk-tui uses an encrypted local store to keep passwords secure.",
	)

	inputW := 40
	if bodyCardWidth-4 < inputW {
		inputW = bodyCardWidth - 4
	}
	inputView := m.theme.InputBox.Width(inputW).Render(m.input.View())

	var errView string
	if m.errMsg != "" {
		errView = lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("⚠ " + m.errMsg)
	}

	cardContent := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		"",
		desc,
		"",
		inputView,
		"",
		errView,
	)

	bodyCard := m.theme.Card.Width(bodyCardWidth).Height(bodyHeight).Render(cardContent)

	footer := m.theme.StatusBar.Width(innerW).Render("enter: unlock  │  ctrl+c / q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyCard, footer)
}
