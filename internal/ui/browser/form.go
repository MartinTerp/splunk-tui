package browser

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/ui/theme"
)

// BookmarkFormResult represents the completed form submission.
type BookmarkFormResult struct {
	OriginalName string
	Bookmark     config.Bookmark
	Password     string
}

// FormModel is the add/edit bookmark modal.
type FormModel struct {
	theme        theme.Theme
	originalName string
	inputs       []textinput.Model
	focusIndex   int
	verifySSL    bool
	width        int
	height       int
	errMsg       string
}

// NewFormModel creates a form for adding or editing an instance bookmark.
func NewFormModel(theme theme.Theme, origName string, b config.Bookmark, initialPass string) FormModel {
	inputs := make([]textinput.Model, 4)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "e.g. prod-search"
	inputs[0].SetValue(b.Name)
	inputs[0].Focus()
	inputs[0].Width = 40

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "e.g. https://splunk.corp:8089"
	inputs[1].SetValue(b.URL)
	inputs[1].Width = 40

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "e.g. admin"
	inputs[2].SetValue(b.Username)
	inputs[2].Width = 40

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "••••••••"
	inputs[3].EchoMode = textinput.EchoPassword
	inputs[3].EchoCharacter = '•'
	inputs[3].SetValue(initialPass)
	inputs[3].Width = 40

	verify := true
	if origName != "" {
		verify = b.VerifySSL
	}

	return FormModel{
		theme:        theme,
		originalName: origName,
		inputs:       inputs,
		focusIndex:   0,
		verifySSL:    verify,
	}
}

func (m FormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m FormModel) Update(msg tea.Msg) (FormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focusIndex = (m.focusIndex + 1) % 6
			return m.updateFocus()
		case "shift+tab", "up":
			m.focusIndex = (m.focusIndex - 1 + 6) % 6
			return m.updateFocus()
		case " ":
			if m.focusIndex == 4 { // SSL toggle
				m.verifySSL = !m.verifySSL
				return m, nil
			}
		case "enter":
			if m.focusIndex == 5 || m.focusIndex == 3 { // Save button or Enter in password
				return m.submit()
			}
			if m.focusIndex == 4 {
				m.verifySSL = !m.verifySSL
				return m, nil
			}
			m.focusIndex = (m.focusIndex + 1) % 6
			return m.updateFocus()
		}
	}

	if m.focusIndex < 4 {
		var cmd tea.Cmd
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m FormModel) updateFocus() (FormModel, tea.Cmd) {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		if i == m.focusIndex {
			cmds[i] = m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return m, tea.Batch(cmds...)
}

func (m FormModel) submit() (FormModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[0].Value())
	rawURL := strings.TrimSpace(m.inputs[1].Value())
	user := strings.TrimSpace(m.inputs[2].Value())
	pass := m.inputs[3].Value()

	if name == "" {
		m.errMsg = "Instance name is required"
		return m, nil
	}
	if rawURL == "" {
		m.errMsg = "URL is required"
		return m, nil
	}
	normalized, err := config.NormalizeURL(rawURL)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	if user == "" {
		m.errMsg = "Username is required"
		return m, nil
	}

	bm := config.Bookmark{
		Name:      name,
		URL:       normalized,
		Username:  user,
		VerifySSL: m.verifySSL,
	}

	return m, func() tea.Msg {
		return BookmarkFormResult{
			OriginalName: m.originalName,
			Bookmark:     bm,
			Password:     pass,
		}
	}
}

func (m FormModel) View() string {
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

	// 1. Header
	titleText := "⚡ Splunk TUI  │  Add Splunk Instance"
	if m.originalName != "" {
		titleText = "⚡ Splunk TUI  │  Edit Instance: " + m.originalName
	}
	header := m.theme.Header.Width(innerW).Render(titleText)

	// 2. Form content
	desc := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(
		"Connect to the Splunk REST API management port (default 8089). Passwords are encrypted in your keyring.",
	)

	labels := []string{"Instance Name:", "URL (with port):", "Username:", "Password:"}
	var fieldRows []string

	for i := 0; i < 4; i++ {
		lbl := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == i).Foreground(m.theme.SubtleText).Render(labels[i])
		box := m.inputs[i].View()
		fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, lbl, " ", box))
	}

	// SSL Verification row
	sslLabel := lipgloss.NewStyle().Width(20).Foreground(m.theme.SubtleText).Render("SSL Verification:")
	sslCheck := "[ ] Insecure (skip certificate check)"
	if m.verifySSL {
		sslCheck = "[x] 🔒 Enabled (strict check)"
	}
	if m.focusIndex == 4 {
		sslCheck = m.theme.ActiveButton.Render(sslCheck)
	} else {
		sslCheck = m.theme.Button.Render(sslCheck)
	}
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, sslLabel, " ", sslCheck))

	// Save button
	saveBtn := "[ Save Bookmark ]"
	if m.focusIndex == 5 {
		saveBtn = m.theme.ActiveButton.Render(saveBtn)
	} else {
		saveBtn = m.theme.Button.Render(saveBtn)
	}
	indent := lipgloss.NewStyle().Width(21).Render("")
	fieldRows = append(fieldRows, "", indent+saveBtn)

	var errView string
	if m.errMsg != "" {
		errView = lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("⚠ " + m.errMsg)
	}

	bodyHeight := h - 4
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	bodyCardWidth := w - 4
	if bodyCardWidth < 20 {
		bodyCardWidth = 20
	}

	formBlock := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		desc,
		"",
		lipgloss.JoinVertical(lipgloss.Left, fieldRows...),
		"",
		errView,
	)

	bodyCard := m.theme.Card.Width(bodyCardWidth).Height(bodyHeight).Render(formBlock)

	// 3. Footer
	footerContent := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(
		"tab / shift+tab: switch field  │  space: toggle ssl  │  enter: save  │  esc: cancel",
	)
	footer := m.theme.StatusBar.Width(innerW).Render(footerContent)

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyCard, footer)
}
