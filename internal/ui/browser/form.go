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
	Password     string // secret: password or token
}

// FormModel is the add/edit bookmark modal.
type FormModel struct {
	theme        theme.Theme
	originalName string
	inputs       []textinput.Model // [0]: name, [1]: url, [2]: username, [3]: password/token
	focusIndex   int               // 0: name, 1: url, 2: authType, 3: user, 4: secret, 5: ssl, 6: save
	isToken      bool
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
	secretVal := initialPass
	if secretVal == "" && b.Token != "" {
		secretVal = b.Token
	}
	inputs[3].SetValue(secretVal)
	inputs[3].Width = 40

	isToken := b.IsTokenAuth()

	verify := true
	if origName != "" {
		verify = b.VerifySSL
	}

	m := FormModel{
		theme:        theme,
		originalName: origName,
		inputs:       inputs,
		focusIndex:   0,
		isToken:      isToken,
		verifySSL:    verify,
	}
	m.updatePlaceholders()
	return m
}

func (m *FormModel) updatePlaceholders() {
	if m.isToken {
		m.inputs[2].Placeholder = "optional display name"
		m.inputs[3].Placeholder = "Splunk Bearer/JWT user token"
	} else {
		m.inputs[2].Placeholder = "e.g. admin"
		m.inputs[3].Placeholder = "••••••••"
	}
}

func (m FormModel) focusedInputIndex() int {
	switch m.focusIndex {
	case 0:
		return 0
	case 1:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	default:
		return -1
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
			m.focusIndex = (m.focusIndex + 1) % 7
			return m.updateFocus()
		case "shift+tab", "up":
			m.focusIndex = (m.focusIndex - 1 + 7) % 7
			return m.updateFocus()
		case " ":
			if m.focusIndex == 2 { // Auth type toggle
				m.isToken = !m.isToken
				m.updatePlaceholders()
				return m, nil
			}
			if m.focusIndex == 5 { // SSL toggle
				m.verifySSL = !m.verifySSL
				return m, nil
			}
		case "enter":
			if m.focusIndex == 6 || m.focusIndex == 4 { // Save button or Enter in secret field
				return m.submit()
			}
			if m.focusIndex == 2 {
				m.isToken = !m.isToken
				m.updatePlaceholders()
				return m, nil
			}
			if m.focusIndex == 5 {
				m.verifySSL = !m.verifySSL
				return m, nil
			}
			m.focusIndex = (m.focusIndex + 1) % 7
			return m.updateFocus()
		}
	}

	if idx := m.focusedInputIndex(); idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m FormModel) updateFocus() (FormModel, tea.Cmd) {
	activeIdx := m.focusedInputIndex()
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		if i == activeIdx {
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

	authType := "password"
	if m.isToken {
		authType = "token"
		if pass == "" && m.originalName == "" {
			m.errMsg = "Auth token is required"
			return m, nil
		}
	} else {
		if user == "" {
			m.errMsg = "Username is required"
			return m, nil
		}
	}

	bm := config.Bookmark{
		Name:      name,
		URL:       normalized,
		Username:  user,
		AuthType:  authType,
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
		"Connect to the Splunk REST API management port (default 8089). Passwords and tokens are encrypted in your keyring.",
	)

	var fieldRows []string

	// Instance Name row (focusIndex 0)
	nameLbl := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 0).Foreground(m.theme.SubtleText).Render("Instance Name:")
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, nameLbl, " ", m.inputs[0].View()))

	// URL row (focusIndex 1)
	urlLbl := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 1).Foreground(m.theme.SubtleText).Render("URL (with port):")
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, urlLbl, " ", m.inputs[1].View()))

	// Auth Method row (focusIndex 2)
	authLabel := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 2).Foreground(m.theme.SubtleText).Render("Auth Method:")
	authTypeView := "[•] Password      ( ) Token"
	if m.isToken {
		authTypeView = "( ) Password      [•] Token (Bearer)"
	}
	if m.focusIndex == 2 {
		authTypeView = m.theme.ActiveButton.Render(authTypeView)
	} else {
		authTypeView = m.theme.Button.Render(authTypeView)
	}
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, authLabel, " ", authTypeView))

	// Username row (focusIndex 3)
	userLabelText := "Username:"
	if m.isToken {
		userLabelText = "Username (optional):"
	}
	userLbl := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 3).Foreground(m.theme.SubtleText).Render(userLabelText)
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, userLbl, " ", m.inputs[2].View()))

	// Secret (Password / Token) row (focusIndex 4)
	secretLabelText := "Password:"
	if m.isToken {
		secretLabelText = "Auth Token:"
	}
	secretLbl := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 4).Foreground(m.theme.SubtleText).Render(secretLabelText)
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, secretLbl, " ", m.inputs[3].View()))

	// SSL Verification row (focusIndex 5)
	sslLabel := lipgloss.NewStyle().Width(20).Bold(m.focusIndex == 5).Foreground(m.theme.SubtleText).Render("SSL Verification:")
	sslCheck := "[ ] Insecure (skip certificate check)"
	if m.verifySSL {
		sslCheck = "[x] 🔒 Enabled (strict check)"
	}
	if m.focusIndex == 5 {
		sslCheck = m.theme.ActiveButton.Render(sslCheck)
	} else {
		sslCheck = m.theme.Button.Render(sslCheck)
	}
	fieldRows = append(fieldRows, lipgloss.JoinHorizontal(lipgloss.Center, sslLabel, " ", sslCheck))

	// Save button (focusIndex 6)
	saveBtn := "[ Save Bookmark ]"
	if m.focusIndex == 6 {
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
		"tab / shift+tab: switch field  │  space: toggle  │  enter: save  │  esc: cancel",
	)
	footer := m.theme.StatusBar.Width(innerW).Render(footerContent)

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyCard, footer)
}
