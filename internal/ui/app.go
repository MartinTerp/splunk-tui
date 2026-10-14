package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/secrets"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/browser"
	"splunk-tui/internal/ui/multiview"
	"splunk-tui/internal/ui/search"
)

type currentScreen int

const (
	screenUnlock currentScreen = iota
	screenBrowser
	screenSearch
	screenMultiView
)

// AppModel is the root Bubble Tea application model.
type AppModel struct {
	theme              Theme
	cfg                *config.Config
	secrets            secrets.Store
	autoInstance       string
	screen             currentScreen
	previousScreen     currentScreen
	unlock             UnlockModel
	browser            browser.BrowserModel
	search             *search.SearchModel
	searchTabs         []*search.SearchModel
	activeTab          int
	currentBookmark    config.Bookmark
	currentClient      *splunk.Client
	currentInfo        *splunk.ServerInfo
	multiView          *multiview.MultiViewModel
	showHelp           bool
	width              int
	height             int
	activeInstanceName string
}

// NewApp creates the root application model.
func NewApp(cfg *config.Config, sec secrets.Store, autoInstance string) AppModel {
	theme := DefaultTheme()
	browserModel := browser.New(theme, cfg, sec)

	initialScreen := screenBrowser
	unlockModel := NewUnlockModel(theme)

	// If using file backend and secrets store requires master password
	if sec != nil && sec.Backend() == "file" {
		// Test if keyring can be accessed
		_, err := sec.Get("__probe__")
		if err == secrets.ErrMasterPasswordRequired {
			initialScreen = screenUnlock
		}
	}

	return AppModel{
		theme:        theme,
		cfg:          cfg,
		secrets:      sec,
		autoInstance: autoInstance,
		screen:       initialScreen,
		unlock:       unlockModel,
		browser:      browserModel,
	}
}

func (m AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	switch m.screen {
	case screenUnlock:
		cmds = append(cmds, m.unlock.Init())
	case screenBrowser:
		cmds = append(cmds, m.browser.Init())
		if m.autoInstance != "" {
			if bm, ok := m.cfg.Find(m.autoInstance); ok {
				var connectCmd tea.Cmd
				m.browser, connectCmd = m.browser.ConnectTo(bm)
				cmds = append(cmds, connectCmd)
			}
		}
	}

	return tea.Batch(cmds...)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.unlock, _ = m.unlock.Update(msg)
		m.browser, _ = m.browser.Update(msg)
		searchH := msg.Height - 1
		if searchH < 1 {
			searchH = msg.Height
		}
		searchSizeMsg := tea.WindowSizeMsg{Width: msg.Width, Height: searchH}
		for i := range m.searchTabs {
			if m.searchTabs[i] != nil {
				*m.searchTabs[i], _ = m.searchTabs[i].Update(searchSizeMsg)
			}
		}
		if m.activeTab >= 0 && m.activeTab < len(m.searchTabs) {
			m.search = m.searchTabs[m.activeTab]
		}
		if m.multiView != nil {
			*m.multiView, _ = m.multiView.Update(msg)
		}
		return m, nil

	case UnlockDoneMsg:
		// Master password provided, reopen secrets store
		opts := secrets.Options{
			Backend:        m.cfg.KeyringBackend,
			MasterPassword: msg.Password,
		}
		sec, err := secrets.Open(opts)
		if err != nil {
			m.unlock.SetError(fmt.Sprintf("Failed to unlock: %v", err))
			return m, nil
		}
		m.secrets = sec
		m.browser = browser.New(m.theme, m.cfg, sec)
		m.screen = screenBrowser

		var autoCmd tea.Cmd
		if m.autoInstance != "" {
			if bm, ok := m.cfg.Find(m.autoInstance); ok {
				m.browser, autoCmd = m.browser.ConnectTo(bm)
			}
		}
		return m, tea.Batch(m.browser.Init(), autoCmd)

	case browser.ConnectedMsg:
		// Transition from browser to search dashboard
		// Reset search GUI state from scratch
		m.currentBookmark = msg.Bookmark
		m.currentClient = msg.Client
		m.currentInfo = msg.Info
		searchMod := search.New(m.theme, m.cfg, msg.Bookmark, msg.Client, msg.Info)
		m.search = &searchMod
		m.searchTabs = []*search.SearchModel{&searchMod}
		m.activeTab = 0
		m.activeInstanceName = msg.Bookmark.Name
		m.screen = screenSearch
		m.previousScreen = screenSearch
		m.browser, _ = m.browser.Update(msg)
		// Send initial window size accounting for tab bar
		w := m.width
		h := m.height
		if w <= 0 {
			w = 80
		}
		if h <= 0 {
			h = 24
		}
		searchH := h - 1
		if searchH < 1 {
			searchH = h
		}
		*m.search, _ = m.search.Update(tea.WindowSizeMsg{Width: w, Height: searchH})
		return m, m.search.Init()

	case browser.MultiConnectedMsg:
		// Transition from browser to Multi View dashboard
		multiMod := multiview.New(m.theme, m.cfg, msg.Connections)
		m.multiView = &multiMod
		m.screen = screenMultiView
		m.previousScreen = screenMultiView
		m.browser, _ = m.browser.Update(msg)
		w := m.width
		h := m.height
		if w <= 0 {
			w = 80
		}
		if h <= 0 {
			h = 24
		}
		*m.multiView, _ = m.multiView.Update(tea.WindowSizeMsg{Width: w, Height: h})
		return m, m.multiView.Init()

	case tea.KeyMsg:
		// Global shortcut: help overlay
		if msg.String() == "?" && !m.isInputFocused() {
			m.showHelp = !m.showHelp
			return m, nil
		}

		if m.showHelp && (msg.String() == "esc" || msg.String() == "q" || msg.String() == "?") {
			m.showHelp = false
			return m, nil
		}

		// Global shortcut: open instance browser
		if msg.String() == "ctrl+o" {
			if m.screen != screenBrowser {
				m.previousScreen = m.screen
				m.screen = screenBrowser
				m.browser.ResetConnecting()
			}
			return m, nil
		}

		// Search Workspace Multi-Tab shortcuts
		if m.screen == screenSearch && len(m.searchTabs) > 0 {
			// Ctrl+T: New Tab
			if msg.String() == "ctrl+t" {
				if m.currentClient != nil {
					newTab := search.New(m.theme, m.cfg, m.currentBookmark, m.currentClient, m.currentInfo)
					w := m.width
					if w <= 0 {
						w = 80
					}
					searchH := m.height - 1
					if searchH < 1 {
						searchH = 24
					}
					newTab, _ = newTab.Update(tea.WindowSizeMsg{Width: w, Height: searchH})
					m.searchTabs = append(m.searchTabs, &newTab)
					m.activeTab = len(m.searchTabs) - 1
					m.search = m.searchTabs[m.activeTab]
					return m, newTab.Init()
				}
				return m, nil
			}

			// Ctrl+W: Close Tab (works from anywhere, even when input is focused)
			if msg.String() == "ctrl+w" {
				if len(m.searchTabs) > 1 {
					if m.activeTab >= 0 && m.activeTab < len(m.searchTabs) && m.searchTabs[m.activeTab] != nil {
						m.searchTabs[m.activeTab].Cancel()
					}
					m.searchTabs = append(m.searchTabs[:m.activeTab], m.searchTabs[m.activeTab+1:]...)
					if m.activeTab >= len(m.searchTabs) {
						m.activeTab = len(m.searchTabs) - 1
					}
					m.search = m.searchTabs[m.activeTab]
				}
				return m, nil
			}

			// Switch tabs with [ / ] (when input is not focused) or Alt+[ / Alt+] / Ctrl+Left / Ctrl+Right
			if msg.String() == "alt+]" || msg.String() == "ctrl+right" || (!m.isInputFocused() && msg.String() == "]") {
				if len(m.searchTabs) > 1 {
					m.activeTab = (m.activeTab + 1) % len(m.searchTabs)
					m.search = m.searchTabs[m.activeTab]
					if m.search != nil {
						m.search.BlurSearch()
					}
				}
				return m, nil
			}
			if msg.String() == "alt+[" || msg.String() == "ctrl+left" || (!m.isInputFocused() && msg.String() == "[") {
				if len(m.searchTabs) > 1 {
					if m.activeTab > 0 {
						m.activeTab--
					} else {
						m.activeTab = len(m.searchTabs) - 1
					}
					m.search = m.searchTabs[m.activeTab]
					if m.search != nil {
						m.search.BlurSearch()
					}
				}
				return m, nil
			}

			// Switch tabs with Alt+1 .. Alt+9
			for i := 1; i <= 9; i++ {
				if msg.String() == fmt.Sprintf("alt+%d", i) {
					idx := i - 1
					if idx < len(m.searchTabs) {
						m.activeTab = idx
						m.search = m.searchTabs[m.activeTab]
					}
					return m, nil
				}
			}
		}

		// If on browser screen and user presses Esc: return to previous active instance/multi-view
		if m.screen == screenBrowser && msg.String() == "esc" && !m.browser.FormIsOpen() {
			if m.previousScreen == screenSearch && m.search != nil {
				m.screen = screenSearch
				return m, nil
			}
			if m.previousScreen == screenMultiView && m.multiView != nil {
				m.screen = screenMultiView
				return m, nil
			}
			if m.search != nil {
				m.screen = screenSearch
				return m, nil
			}
			if m.multiView != nil {
				m.screen = screenMultiView
				return m, nil
			}
		}

		// Global quit
		if (msg.String() == "q" && !m.isInputFocused()) || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	// Delegate to active screen
	switch m.screen {
	case screenUnlock:
		var cmd tea.Cmd
		m.unlock, cmd = m.unlock.Update(msg)
		return m, cmd

	case screenBrowser:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		return m, cmd

	case screenSearch:
		if len(m.searchTabs) > 0 {
			if _, isKey := msg.(tea.KeyMsg); isKey {
				if m.activeTab >= 0 && m.activeTab < len(m.searchTabs) && m.searchTabs[m.activeTab] != nil {
					var cmd tea.Cmd
					*m.searchTabs[m.activeTab], cmd = m.searchTabs[m.activeTab].Update(msg)
					m.search = m.searchTabs[m.activeTab]
					return m, cmd
				}
			} else {
				var cmds []tea.Cmd
				for i := range m.searchTabs {
					if m.searchTabs[i] != nil {
						var cmd tea.Cmd
						*m.searchTabs[i], cmd = m.searchTabs[i].Update(msg)
						if cmd != nil {
							cmds = append(cmds, cmd)
						}
					}
				}
				if m.activeTab >= 0 && m.activeTab < len(m.searchTabs) {
					m.search = m.searchTabs[m.activeTab]
				}
				return m, tea.Batch(cmds...)
			}
		} else if m.search != nil {
			var cmd tea.Cmd
			*m.search, cmd = m.search.Update(msg)
			return m, cmd
		}

	case screenMultiView:
		if m.multiView != nil {
			var cmd tea.Cmd
			*m.multiView, cmd = m.multiView.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m AppModel) isInputFocused() bool {
	if m.screen == screenSearch && m.search != nil {
		return m.search.SearchBarIsFocused()
	}
	if m.screen == screenMultiView && m.multiView != nil {
		return m.multiView.SearchBarIsFocused()
	}
	return false
}

func (m AppModel) View() string {
	if m.showHelp {
		return m.renderHelpView()
	}

	switch m.screen {
	case screenUnlock:
		return m.unlock.View()
	case screenBrowser:
		return m.browser.View()
	case screenSearch:
		if m.search != nil {
			tabBar := m.renderTabBar(m.width)
			if tabBar != "" {
				return lipgloss.JoinVertical(lipgloss.Left, tabBar, m.search.View())
			}
			return m.search.View()
		}
	case screenMultiView:
		if m.multiView != nil {
			return m.multiView.View()
		}
	}

	return ""
}

func (m AppModel) renderTabBar(width int) string {
	if width <= 0 {
		width = 80
	}
	if len(m.searchTabs) == 0 {
		return ""
	}

	var tabItems []string
	for i, tab := range m.searchTabs {
		if tab == nil {
			continue
		}
		title := tab.TabTitle()
		status := ""
		if tab.IsSearching() {
			status = " ⚡"
		}

		tabLabel := fmt.Sprintf("%d: %s%s", i+1, title, status)
		if i == m.activeTab {
			activeStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(m.theme.Primary).
				Padding(0, 1)
			tabItems = append(tabItems, activeStyle.Render(tabLabel))
		} else {
			inactiveStyle := lipgloss.NewStyle().
				Foreground(m.theme.SubtleText).
				Background(m.theme.Surface).
				Padding(0, 1)
			tabItems = append(tabItems, inactiveStyle.Render(tabLabel))
		}
	}

	newTabBtn := lipgloss.NewStyle().
		Foreground(m.theme.Highlight).
		Background(m.theme.Surface).
		Padding(0, 1).
		Render("+ New (Ctrl+T)")
	tabItems = append(tabItems, newTabBtn)

	leftTabs := lipgloss.JoinHorizontal(lipgloss.Top, tabItems...)

	hintText := "Ctrl+T new │ Ctrl+W close │ Alt+1..9 switch"
	rightHint := lipgloss.NewStyle().
		Foreground(m.theme.SubtleText).
		Render(hintText)

	leftW := lipgloss.Width(leftTabs)
	rightW := lipgloss.Width(rightHint)
	gapW := width - leftW - rightW
	if gapW < 1 {
		gapW = 1
		rightHint = ""
	}

	spaceFill := lipgloss.NewStyle().
		Background(m.theme.Background).
		Width(gapW).
		Render(strings.Repeat(" ", gapW))

	line := lipgloss.JoinHorizontal(lipgloss.Center, leftTabs, spaceFill, rightHint)
	return lipgloss.NewStyle().
		Background(m.theme.Background).
		Width(width).
		MaxHeight(1).
		Render(line)
}

func (m AppModel) renderHelpView() string {
	boxWidth := 100
	boxHeight := 24

	title := m.theme.Title.Render("📖 splunk-tui Keyboard Shortcuts")

	shortcuts := []struct{ key, desc string }{
		{"/", "Focus search query input (or filter in browser)"},
		{"Ctrl+F", "Focus search query input from anywhere"},
		{"Enter", "Run search (in input) / Inspect event (in table)"},
		{"Ctrl+T", "New search workspace tab"},
		{"Ctrl+W", "Close current search tab (from anywhere)"},
		{"Alt+1..9", "Jump directly to search tab 1..9"},
		{"[ / ]", "Switch between search tabs"},
		{"< / >", "Scrub timeline left / right"},
		{"z / Z", "Zoom in to scrubbed timeline slice"},
		{"Ctrl+K / c", "Browse Splunk configuration (.conf files)"},
		{"Ctrl+X", "Cancel running search job"},
		{"Ctrl+G", "Toggle collapse/expand chart pane"},
		{"v", "Cycle visualization chart type (Bar/Pie/Col, Line/Spark)"},
		{"h / Ctrl+H", "Cycle chart height (Small → Med → Lrg → Full)"},
		{"← / →", "Scroll long events / table horizontally (h / l)"},
		{"Tab", "Cycle focus (search bar ⇄ fields ⇄ table / grid)"},
		{"Space", "Toggle instance selection in browser (Multi View)"},
		{"Ctrl+A", "Select all instances in browser"},
		{"n / PgDn", "Next page of results"},
		{"p / PgUp", "Previous page of results"},
		{"+", "Include field value in base search"},
		{"-", "Exclude field value in base search"},
		{"s", "Toggle field as table column"},
		{"Ctrl+O", "Open instance browser to switch servers"},
		{"Esc", "Return to active instance (from browser)"},
		{"?", "Toggle this help dialog"},
		{"q / Ctrl+C", "Quit application"},
	}

	var rows []string
	for _, sc := range shortcuts {
		k := m.theme.HelpKey.Width(14).Render(sc.key)
		d := m.theme.HelpDesc.Render(sc.desc)
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, k, " ", d))
	}

	closeNotice := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Press ? or Esc to close help")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		"",
		closeNotice,
	)

	card := m.theme.Card.Width(boxWidth).Height(boxHeight).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
}
