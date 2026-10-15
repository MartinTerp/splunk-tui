package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/secrets"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

// ConnectedMsg is emitted when a connection to a Splunk instance succeeds.
type ConnectedMsg struct {
	Bookmark config.Bookmark
	Client   *splunk.Client
	Info     *splunk.ServerInfo
}

// InstanceConnection holds an active connection and info for a Splunk instance in Multi View.
type InstanceConnection struct {
	Bookmark config.Bookmark
	Client   *splunk.Client
	Info     *splunk.ServerInfo
	Err      error
}

// MultiConnectedMsg is emitted when multi-instance connections complete.
type MultiConnectedMsg struct {
	Connections []InstanceConnection
}

type connectErrMsg struct {
	bookmark config.Bookmark
	err      error
}

// ConnectionErrorInfo holds diagnostic details for connection failures.
type ConnectionErrorInfo struct {
	Bookmark   config.Bookmark
	Title      string
	Reason     string
	Action     string
	RawDetails string
}

// BrowserModel manages instance selection and bookmark management.
type BrowserModel struct {
	theme              theme.Theme
	cfg                *config.Config
	secrets            secrets.Store
	cursor             int
	selected           map[string]bool // multi-selected instances for Multi View
	connectedInstances map[string]bool // currently connected instance(s)
	filterInput        textinput.Model
	filtering          bool
	connecting         bool
	connectingName     string
	connectingURL      string
	spinner            spinner.Model
	form               *FormModel
	errMsg             string
	errInfo            *ConnectionErrorInfo
	width              int
	height             int
}

// New creates an instance browser model.
func New(th theme.Theme, cfg *config.Config, sec secrets.Store) BrowserModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle() // clean plain text frames without ANSI codes

	ti := textinput.New()
	ti.Placeholder = "Type to filter name or URL... (/ to filter, esc to clear)"
	ti.Prompt = "🔍 "
	ti.CharLimit = 64

	return BrowserModel{
		theme:              th,
		cfg:                cfg,
		secrets:            sec,
		selected:           make(map[string]bool),
		connectedInstances: make(map[string]bool),
		filterInput:        ti,
		spinner:            s,
	}
}

// filteredBookmarks returns the subset of bookmarks matching the active filter query.
func (m BrowserModel) filteredBookmarks() []config.Bookmark {
	if m.cfg == nil {
		return nil
	}
	q := strings.TrimSpace(strings.ToLower(m.filterInput.Value()))
	if q == "" {
		return m.cfg.Bookmarks
	}
	var filtered []config.Bookmark
	for _, bm := range m.cfg.Bookmarks {
		name := strings.ToLower(bm.Name)
		u, _ := bm.NormalizedURL()
		urlStr := strings.ToLower(u)
		if strings.Contains(name, q) || strings.Contains(urlStr, q) {
			filtered = append(filtered, bm)
		}
	}
	return filtered
}

func (m BrowserModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m BrowserModel) Update(msg tea.Msg) (BrowserModel, tea.Cmd) {
	// If form is active, delegate to it
	if m.form != nil {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "esc" {
				m.form = nil
				return m, nil
			}
		case BookmarkFormResult:
			// Save bookmark to config
			if err := m.cfg.Upsert(msg.OriginalName, msg.Bookmark); err != nil {
				m.errMsg = err.Error()
				m.errInfo = nil
				m.form = nil
				return m, nil
			}
			if err := m.cfg.Save(); err != nil {
				m.errMsg = fmt.Sprintf("saving config: %v", err)
			}
			// Handle secret rename if instance name was changed
			if msg.OriginalName != "" && msg.OriginalName != msg.Bookmark.Name && m.secrets != nil {
				_ = m.secrets.Rename(msg.OriginalName, msg.Bookmark.Name)
			}
			// Save password in secrets if provided
			if msg.Password != "" && m.secrets != nil {
				if err := m.secrets.Set(msg.Bookmark.Name, msg.Password); err != nil {
					m.errMsg = fmt.Sprintf("saving password: %v", err)
				}
			}
			m.errMsg = ""
			m.errInfo = nil
			m.form = nil
			return m, nil
		}

		var cmd tea.Cmd
		*m.form, cmd = m.form.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case ConnectedMsg:
		m.connecting = false
		m.connectingName = ""
		m.connectingURL = ""
		m.errMsg = ""
		m.errInfo = nil
		m.connectedInstances = map[string]bool{msg.Bookmark.Name: true}
		return m, nil

	case MultiConnectedMsg:
		m.connecting = false
		m.connectingName = ""
		m.connectingURL = ""
		m.errMsg = ""
		m.errInfo = nil
		m.connectedInstances = make(map[string]bool)
		for _, c := range msg.Connections {
			if c.Err == nil {
				m.connectedInstances[c.Bookmark.Name] = true
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.form != nil {
			*m.form, _ = m.form.Update(msg)
		}
		return m, nil

	case spinner.TickMsg:
		if m.connecting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case connectErrMsg:
		m.connecting = false
		m.errInfo = buildConnectionErrorInfo(msg.err, msg.bookmark)
		if m.errInfo != nil {
			m.errMsg = m.errInfo.Reason
		}
		return m, nil

	case tea.KeyMsg:
		if m.connecting {
			return m, nil
		}

		if m.filtering {
			switch msg.String() {
			case "esc":
				m.filtering = false
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				return m, nil

			case "enter":
				m.filtering = false
				m.filterInput.Blur()
				bms := m.filteredBookmarks()
				if len(bms) > 0 && m.cursor < len(bms) {
					return m.ConnectTo(bms[m.cursor])
				}
				return m, nil

			case "down":
				bms := m.filteredBookmarks()
				if m.cursor < len(bms)-1 {
					m.cursor++
				}
				return m, nil

			case "up":
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil

			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				bms := m.filteredBookmarks()
				if m.cursor >= len(bms) && len(bms) > 0 {
					m.cursor = len(bms) - 1
				}
				return m, cmd
			}
		}

		switch msg.String() {
		case "/":
			m.filtering = true
			m.filterInput.Focus()
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.errMsg = ""
				m.errInfo = nil
			}
			return m, nil

		case "down", "j":
			bms := m.filteredBookmarks()
			if m.cursor < len(bms)-1 {
				m.cursor++
				m.errMsg = ""
				m.errInfo = nil
			}
			return m, nil

		case " ":
			bms := m.filteredBookmarks()
			if len(bms) > 0 && m.cursor < len(bms) {
				bm := bms[m.cursor]
				if m.selected[bm.Name] {
					delete(m.selected, bm.Name)
				} else {
					m.selected[bm.Name] = true
				}
			}
			return m, nil

		case "ctrl+a":
			bms := m.filteredBookmarks()
			if len(m.selected) == len(bms) && len(bms) > 0 {
				m.selected = make(map[string]bool)
			} else {
				for _, bm := range bms {
					m.selected[bm.Name] = true
				}
			}
			return m, nil

		case "enter":
			bms := m.filteredBookmarks()
			if len(bms) == 0 {
				if len(m.cfg.Bookmarks) == 0 {
					form := NewFormModel(m.theme, "", config.Bookmark{}, "")
					form.width = m.width
					form.height = m.height
					m.form = &form
					return m, m.form.Init()
				}
				return m, nil
			}
			sel := m.selectedBookmarks()
			if len(sel) >= 2 {
				return m.ConnectMulti(sel)
			} else if len(sel) == 1 {
				return m.ConnectTo(sel[0])
			}
			if m.cursor < len(bms) {
				bm := bms[m.cursor]
				return m.ConnectTo(bm)
			}
			return m, nil

		case "a":
			m.errMsg = ""
			m.errInfo = nil
			form := NewFormModel(m.theme, "", config.Bookmark{}, "")
			form.width = m.width
			form.height = m.height
			m.form = &form
			return m, m.form.Init()

		case "e":
			bms := m.filteredBookmarks()
			if len(bms) > 0 && m.cursor < len(bms) {
				m.errMsg = ""
				m.errInfo = nil
				bm := bms[m.cursor]
				pass := ""
				if m.secrets != nil {
					pass, _ = m.secrets.Get(bm.Name)
				}
				form := NewFormModel(m.theme, bm.Name, bm, pass)
				form.width = m.width
				form.height = m.height
				m.form = &form
				return m, m.form.Init()
			}

		case "d":
			bms := m.filteredBookmarks()
			if len(bms) > 0 && m.cursor < len(bms) {
				m.errMsg = ""
				m.errInfo = nil
				bm := bms[m.cursor]
				_ = m.cfg.Delete(bm.Name)
				_ = m.cfg.Save()
				if m.secrets != nil {
					_ = m.secrets.Delete(bm.Name)
				}
				if m.cursor >= len(m.filteredBookmarks()) && m.cursor > 0 {
					m.cursor--
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// ConnectTo begins an asynchronous connection attempt to the given instance.
func (m BrowserModel) ConnectTo(bm config.Bookmark) (BrowserModel, tea.Cmd) {
	m.connecting = true
	m.connectingName = bm.Name
	m.connectingURL = bm.URL
	m.errMsg = ""
	m.errInfo = nil

	sec := m.secrets
	connectCmd := func() tea.Msg {
		var secret string
		if sec != nil {
			type secretResult struct {
				val string
				err error
			}
			secCh := make(chan secretResult, 1)
			go func() {
				p, err := sec.Get(bm.Name)
				secCh <- secretResult{val: p, err: err}
			}()

			select {
			case res := <-secCh:
				if res.err == nil {
					secret = res.val
				}
			case <-time.After(2 * time.Second):
				return connectErrMsg{
					bookmark: bm,
					err:      fmt.Errorf("keyring retrieval timed out (keyring daemon unresponsive)"),
				}
			}
		}

		client, err := createClientForBookmark(bm, secret, m.cfg.APITimeout())
		if err != nil {
			return connectErrMsg{bookmark: bm, err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), m.cfg.APITimeout())
		defer cancel()

		if err := client.Login(ctx); err != nil {
			return connectErrMsg{bookmark: bm, err: err}
		}

		info, err := client.GetServerInfo(ctx)
		if err != nil {
			return connectErrMsg{bookmark: bm, err: err}
		}

		return ConnectedMsg{
			Bookmark: bm,
			Client:   client,
			Info:     info,
		}
	}

	return m, tea.Batch(m.spinner.Tick, connectCmd)
}

func createClientForBookmark(bm config.Bookmark, secret string, timeout time.Duration) (*splunk.Client, error) {
	normURL, err := bm.NormalizedURL()
	if err != nil {
		return nil, err
	}

	clientCfg := splunk.ClientConfig{
		BaseURL:   normURL,
		Username:  bm.Username,
		VerifySSL: bm.VerifySSL,
		Timeout:   timeout,
	}

	if bm.IsTokenAuth() {
		token := bm.Token
		if token == "" {
			token = secret
		}
		if token == "" {
			if envToken := os.Getenv("SPLUNK_TOKEN"); envToken != "" {
				token = envToken
			} else if envToken := os.Getenv("SPLUNK_TUI_TOKEN"); envToken != "" {
				token = envToken
			}
		}
		if token == "" {
			return nil, fmt.Errorf("no token found for %q", bm.Name)
		}
		clientCfg.Token = token
		clientCfg.AuthType = "token"
	} else {
		password := secret
		if password == "" {
			if envPass := os.Getenv("SPLUNK_PASSWORD"); envPass != "" {
				password = envPass
			} else if envPass := os.Getenv("SPLUNK_TUI_PASSWORD"); envPass != "" {
				password = envPass
			}
		}
		if bm.Username != "" && password == "" {
			return nil, fmt.Errorf("no password found for %q", bm.Name)
		}
		clientCfg.Password = password
		clientCfg.AuthType = "password"
	}

	return splunk.NewClient(clientCfg)
}

func (m BrowserModel) selectedBookmarks() []config.Bookmark {
	var bms []config.Bookmark
	for _, bm := range m.cfg.Bookmarks {
		if m.selected[bm.Name] {
			bms = append(bms, bm)
		}
	}
	return bms
}

func (m BrowserModel) selectedCount() int {
	return len(m.selected)
}

// ResetConnecting clears connecting state and resets status to idle.
func (m *BrowserModel) ResetConnecting() {
	m.connecting = false
	m.connectingName = ""
	m.connectingURL = ""
}

// FormIsOpen reports whether the bookmark add/edit modal is currently active.
func (m BrowserModel) FormIsOpen() bool {
	return m.form != nil
}

// SetConnected marks a specific instance as active/connected.
func (m *BrowserModel) SetConnected(name string) {
	m.connecting = false
	m.connectedInstances = map[string]bool{name: true}
}

// ConnectedInstances returns the set of currently connected instance names.
func (m BrowserModel) ConnectedInstances() map[string]bool {
	return m.connectedInstances
}

// ConnectMulti begins asynchronous concurrent connection attempts to multiple instances.
func (m BrowserModel) ConnectMulti(bookmarks []config.Bookmark) (BrowserModel, tea.Cmd) {
	m.connecting = true
	m.connectingName = fmt.Sprintf("%d instances", len(bookmarks))
	m.connectingURL = "Multi View"
	m.errMsg = ""
	m.errInfo = nil

	sec := m.secrets
	connectCmd := func() tea.Msg {
		var wg sync.WaitGroup
		results := make([]InstanceConnection, len(bookmarks))

		for i, bm := range bookmarks {
			wg.Add(1)
			go func(idx int, b config.Bookmark) {
				defer wg.Done()

				var secret string
				if sec != nil {
					p, _ := sec.Get(b.Name)
					secret = p
				}

				client, err := createClientForBookmark(b, secret, m.cfg.APITimeout())
				if err != nil {
					results[idx] = InstanceConnection{Bookmark: b, Err: err}
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), m.cfg.APITimeout())
				defer cancel()

				if err := client.Login(ctx); err != nil {
					results[idx] = InstanceConnection{Bookmark: b, Client: client, Err: err}
					return
				}

				info, err := client.GetServerInfo(ctx)
				if err != nil {
					results[idx] = InstanceConnection{Bookmark: b, Client: client, Err: err}
					return
				}

				results[idx] = InstanceConnection{Bookmark: b, Client: client, Info: info}
			}(i, bm)
		}

		wg.Wait()
		return MultiConnectedMsg{Connections: results}
	}

	return m, tea.Batch(m.spinner.Tick, connectCmd)
}

func (m BrowserModel) View() string {
	if m.form != nil {
		return m.form.View()
	}

	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	// Content width inside padded Header and StatusBar (Padding(0, 1) = 2 columns padding)
	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}

	// 1. Top Header Bar (full width w)
	headerTitle := "⚡ Splunk TUI  │  Instance Browser"
	keyringBackend := "auto"
	if m.secrets != nil {
		keyringBackend = m.secrets.Backend()
	}
	backendInfo := fmt.Sprintf("Keyring: %s  •  Instances: %d", keyringBackend, len(m.cfg.Bookmarks))
	space := innerW - lipgloss.Width(headerTitle) - lipgloss.Width(backendInfo)
	if space < 2 {
		space = 2
	}
	headerText := headerTitle + strings.Repeat(" ", space) + backendInfo
	header := m.theme.Header.Width(w).MaxHeight(1).Render(headerText)

	// 2. Body Area inside Card
	bodyCardWidth := w - 4 // border (2) + padding (2)
	if bodyCardWidth < 20 {
		bodyCardWidth = 20
	}
	bodyHeight := h - 4 // header (1) + footer (1) + card borders (2)
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	var bodyContent string

	if len(m.cfg.Bookmarks) == 0 {
		subTitle := lipgloss.NewStyle().
			Foreground(m.theme.Primary).
			Bold(true).
			Render("No Splunk instances configured yet.")

		actionPrompt := lipgloss.NewStyle().
			Foreground(m.theme.Highlight).
			Bold(true).
			Render("▶ Press 'a' to add your first Splunk bookmark")

		guideDesc := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(
			"splunk-tui connects directly to the Splunk REST management port (default 8089).\n" +
				"Credentials are encrypted securely using your system keyring.",
		)

		details := []string{
			"Required connection parameters:",
			"  • Name:        Friendly display name (e.g., 'production', 'dev-cluster')",
			"  • URL:         Host with port (e.g., 'https://splunk.corp:8089')",
			"  • Username:    Splunk user account with REST search capabilities",
			"  • Password:    Splunk user password or API token",
			"  • SSL:         Enable or disable certificate verification",
		}
		detailBlock := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(strings.Join(details, "\n"))

		bodyContent = lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			subTitle,
			"",
			actionPrompt,
			"",
			guideDesc,
			"",
			detailBlock,
		)
	} else {
		contentWidth := bodyCardWidth - 2
		if contentWidth < 18 {
			contentWidth = 18
		}

		nameW := 20
		userW := 14
		sslW := 8
		statusW := 14
		prefixW := 6 // "▶ [✓] "
		urlW := contentWidth - (nameW + userW + sslW + statusW + prefixW + 5)
		if urlW < 14 {
			urlW = 14
		}

		filterBar := m.filterInput.View()
		filterDivider := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render(strings.Repeat("─", contentWidth))

		hdrText := fmt.Sprintf("      %-*s %-*s %-*s %-*s %-*s", nameW, "NAME", urlW, "URL", userW, "USERNAME", sslW, "SSL", statusW, "STATUS")
		tableHeader := m.theme.TableHeader.Render(hdrText)
		divider := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render(strings.Repeat("─", contentWidth))

		var rows []string
		rows = append(rows, filterBar, filterDivider, tableHeader, divider)

		bms := m.filteredBookmarks()
		if len(bms) == 0 {
			noMatches := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render(fmt.Sprintf("  No instances matching filter %q. Press Esc to clear.", m.filterInput.Value()))
			rows = append(rows, "", noMatches)
		} else {
			for i, bm := range bms {
				cursorSymbol := "  "
				isCurrent := i == m.cursor
				if isCurrent {
					cursorSymbol = "▶ "
				}

				checkSymbol := "[ ] "
				if m.selected[bm.Name] {
					checkSymbol = "[✓] "
				}

				normURL, _ := bm.NormalizedURL()

				sslBadge := "🔒"
				if !bm.VerifySSL {
					sslBadge = "⚠"
				}

				nameCol := truncateText(bm.Name, nameW)
				nameColPadded := nameCol + strings.Repeat(" ", nameW-lipgloss.Width(nameCol))

				urlCol := truncateText(normURL, urlW)
				urlColPadded := urlCol + strings.Repeat(" ", urlW-lipgloss.Width(urlCol))

				userDisplay := bm.Username
				if bm.IsTokenAuth() {
					if userDisplay == "" {
						userDisplay = "[token]"
					} else {
						userDisplay = userDisplay + " [token]"
					}
				}
				userCol := truncateText(userDisplay, userW)
				userColPadded := userCol + strings.Repeat(" ", userW-lipgloss.Width(userCol))

				sslCol := sslBadge
				if pad := sslW - lipgloss.Width(sslBadge); pad > 0 {
					sslCol = sslBadge + strings.Repeat(" ", pad)
				}

				statusBadge := ""
				if m.connectedInstances[bm.Name] {
					statusBadge = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render("✔ connected")
				}
				statusPadded := statusBadge
				if pad := statusW - lipgloss.Width(statusBadge); pad > 0 {
					statusPadded += strings.Repeat(" ", pad)
				}

				if isCurrent {
					cursorStyled := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(cursorSymbol)
					checkStyled := checkSymbol
					if m.selected[bm.Name] {
						checkStyled = lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render(checkSymbol)
					}
					nameStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(nameColPadded)
					urlStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(urlColPadded)
					userStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(userColPadded)

					line := fmt.Sprintf("%s%s%s %s %s %s %s", cursorStyled, checkStyled, nameStyled, urlStyled, userStyled, sslCol, statusPadded)
					rows = append(rows, line)
				} else {
					checkStyled := checkSymbol
					if m.selected[bm.Name] {
						checkStyled = lipgloss.NewStyle().Foreground(m.theme.Highlight).Render(checkSymbol)
					}
					nameStyled := lipgloss.NewStyle().Foreground(m.theme.Text).Render(nameColPadded)
					urlStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(urlColPadded)
					userStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(userColPadded)

					line := fmt.Sprintf("%s%s%s %s %s %s %s", cursorSymbol, checkStyled, nameStyled, urlStyled, userStyled, sslCol, statusPadded)
					rows = append(rows, line)
				}
			}
		}

		// If connecting or connection failed, display prominent status card inside the main card
		if m.connecting {
			spinText := fmt.Sprintf("%s Connecting to %s (%s)...", m.spinner.View(), m.connectingName, m.connectingURL)
			spinBox := lipgloss.NewStyle().
				Foreground(m.theme.Primary).
				Bold(true).
				Padding(1, 1).
				Render(spinText)
			rows = append(rows, "", spinBox)
		} else if m.errInfo != nil {
			errBoxWidth := bodyCardWidth - 2
			if errBoxWidth < 30 {
				errBoxWidth = 30
			}

			titleStyle := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true)
			lblStyle := lipgloss.NewStyle().Width(10).Foreground(m.theme.SubtleText)
			valStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
			actionStyle := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
			rawStyle := lipgloss.NewStyle().Foreground(m.theme.SubtleText)

			targetStr := m.errInfo.Bookmark.Name
			if m.errInfo.Bookmark.URL != "" {
				targetStr += " (" + m.errInfo.Bookmark.URL + ")"
			}

			errLines := []string{
				titleStyle.Render("⚠ " + m.errInfo.Title),
				"",
				lblStyle.Render("Target:") + " " + valStyle.Render(targetStr),
				lblStyle.Render("Reason:") + " " + valStyle.Render(m.errInfo.Reason),
				lblStyle.Render("Action:") + " " + actionStyle.Render(m.errInfo.Action),
			}

			if m.errInfo.RawDetails != "" && m.errInfo.RawDetails != m.errInfo.Reason {
				errLines = append(errLines, "", lblStyle.Render("Details:")+" "+rawStyle.Render(truncateText(m.errInfo.RawDetails, errBoxWidth-14)))
			}

			contentW := errBoxWidth - 4
			if contentW < 20 {
				contentW = 20
			}
			errBoxStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(m.theme.Error).
				Padding(0, 1).
				Width(contentW)

			errBox := errBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, errLines...))
			rows = append(rows, "", errBox)
		}

		bodyContent = lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	bodyCard := m.theme.Card.Width(bodyCardWidth).Height(bodyHeight).MaxHeight(h - 2).Render(bodyContent)

	// 3. Bottom Footer / Status Bar (full width w)
	var leftStatusText string
	var leftStatusStyle lipgloss.Style
	selCount := m.selectedCount()

	type hintTiers struct {
		full  string
		short string
		min   string
	}
	var hints hintTiers

	if m.connecting {
		leftStatusText = fmt.Sprintf("%s Connecting to %s (%s)...", m.spinner.View(), m.connectingName, m.connectingURL)
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true)
		hints = hintTiers{full: "connecting...", short: "connecting...", min: ""}
	} else if m.errMsg != "" {
		leftStatusText = "⚠ " + m.errMsg
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true)
		hints = hintTiers{
			full:  "enter: retry  │  e: edit  │  q: quit",
			short: "enter: retry  │  q: quit",
			min:   "q: quit",
		}
	} else if selCount >= 2 {
		leftStatusText = fmt.Sprintf("⚡ %d instances selected (Multi View)", selCount)
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
		hints = hintTiers{
			full:  "enter: connect multi view  │  space: uncheck  │  ctrl+a: all  │  q: quit",
			short: "enter: connect  │  space: uncheck  │  q: quit",
			min:   "enter: connect",
		}
	} else if selCount == 1 {
		leftStatusText = "1 instance selected"
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Highlight)
		hints = hintTiers{
			full:  "enter: connect  │  space: uncheck  │  a: add  │  e: edit  │  q: quit",
			short: "enter: connect  │  space: uncheck  │  q: quit",
			min:   "enter: connect",
		}
	} else if len(m.connectedInstances) > 0 {
		var activeNames []string
		for _, bm := range m.cfg.Bookmarks {
			if m.connectedInstances[bm.Name] {
				activeNames = append(activeNames, bm.Name)
			}
		}
		if len(activeNames) == 1 {
			leftStatusText = fmt.Sprintf("✔ Connected: %s (esc to resume)", activeNames[0])
		} else {
			leftStatusText = fmt.Sprintf("✔ Connected: %d instances (esc to resume)", len(activeNames))
		}
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true)
		hints = hintTiers{
			full:  "esc: resume  │  /: filter  │  enter: switch  │  space: select  │  a: add  │  q: quit",
			short: "esc: resume  │  enter: switch  │  a: add  │  q: quit",
			min:   "esc: resume  │  q: quit",
		}
	} else {
		leftStatusText = "Ready"
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.SubtleText)
		hints = hintTiers{
			full:  "/: filter  │  enter: connect  │  space: select  │  a: add  │  e: edit  │  d: delete  │  q: quit",
			short: "/: filter  │  enter: connect  │  a: add  │  q: quit",
			min:   "enter: connect  │  q: quit",
		}
	}

	if m.filtering {
		leftStatusText = fmt.Sprintf("🔍 Filtering: %q (%d matches)", m.filterInput.Value(), len(m.filteredBookmarks()))
		leftStatusStyle = lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
		hints = hintTiers{
			full:  "enter: connect  │  esc: clear  │  up/down: navigate",
			short: "enter: connect  │  esc: clear",
			min:   "esc: clear",
		}
	}

	availWidth := innerW - 2
	if availWidth < 10 {
		availWidth = 10
	}

	availForHints := availWidth - lipgloss.Width(leftStatusText) - 2
	var rightHelpText string
	if hints.full != "" && lipgloss.Width(hints.full) <= availForHints {
		rightHelpText = hints.full
	} else if hints.short != "" && lipgloss.Width(hints.short) <= availForHints {
		rightHelpText = hints.short
	} else if hints.min != "" && lipgloss.Width(hints.min) <= availForHints {
		rightHelpText = hints.min
	} else {
		rightHelpText = ""
	}

	maxLeft := availWidth - lipgloss.Width(rightHelpText) - 1
	if maxLeft < 10 {
		maxLeft = 10
	}
	if lipgloss.Width(leftStatusText) > maxLeft {
		leftStatusText = truncateText(leftStatusText, maxLeft)
	}

	leftStyled := leftStatusStyle.Render(leftStatusText)
	var rightStyled string
	if rightHelpText != "" {
		rightStyled = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(rightHelpText)
	}

	gap := availWidth - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
	if gap < 1 {
		gap = 1
	}
	barContent := leftStyled + strings.Repeat(" ", gap) + rightStyled
	footer := m.theme.StatusBar.Width(w).MaxHeight(1).Render(barContent)

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyCard, footer)
}

func buildConnectionErrorInfo(err error, bm config.Bookmark) *ConnectionErrorInfo {
	if err == nil {
		return nil
	}
	s := err.Error()
	info := &ConnectionErrorInfo{
		Bookmark:   bm,
		RawDetails: s,
	}

	switch {
	case strings.Contains(s, "no password found"):
		info.Title = "Missing Credentials"
		info.Reason = fmt.Sprintf("No password found in keyring for instance %q", bm.Name)
		info.Action = "Press 'e' to edit this bookmark and set the Splunk password"

	case strings.Contains(s, "keyring retrieval timed out"):
		info.Title = "Keyring Daemon Timeout"
		info.Reason = "System keyring service did not respond within 2 seconds"
		info.Action = "Check your desktop keyring daemon or start splunk-tui with --keyring-backend=file"

	case strings.Contains(s, "certificate signed by unknown authority") || strings.Contains(s, "x509") || strings.Contains(s, "unknown authority"):
		info.Title = "SSL Certificate Verification Failed"
		info.Reason = "Splunk uses a self-signed certificate untrusted by your operating system"
		info.Action = "Press 'e' to edit bookmark and toggle Insecure SSL, then retry"

	case strings.Contains(s, "connection refused"):
		info.Title = "Connection Refused"
		info.Reason = fmt.Sprintf("Port refused connection at %s", bm.URL)
		info.Action = "Check if Splunk is running and verify the port (Splunk REST API default is 8089)"

	case strings.Contains(s, "no such host"):
		info.Title = "Host Not Found"
		info.Reason = fmt.Sprintf("DNS failed to resolve hostname for %s", bm.URL)
		info.Action = "Press 'e' to check hostname spelling or verify your network DNS / hosts file"

	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "timeout"):
		info.Title = "Connection Timed Out"
		info.Reason = fmt.Sprintf("Server at %s did not respond within 5 seconds", bm.URL)
		info.Action = "Check network reachability, VPN status, or firewall rules for this port"

	case strings.Contains(s, "401") || strings.Contains(s, "Unauthorized") || strings.Contains(s, "Login failed"):
		info.Title = "Authentication Failed"
		if bm.IsTokenAuth() {
			info.Reason = fmt.Sprintf("Invalid or expired token for %q", bm.Name)
			info.Action = "Press 'e' to update your Splunk API token"
		} else {
			info.Reason = fmt.Sprintf("Invalid username or password for %q", bm.Name)
			info.Action = "Press 'e' to update your Splunk credentials"
		}

	case strings.Contains(s, "404") || strings.Contains(s, "Not Found"):
		info.Title = "Splunk REST API Not Found (404)"
		info.Reason = fmt.Sprintf("Endpoint /services/auth/login was not found on %s", bm.URL)
		info.Action = "Check port: Splunk Web UI is port 8000, but REST management API is port 8089"

	default:
		info.Title = "Connection Failed"
		info.Reason = s
		info.Action = "Press 'e' to verify bookmark configuration, or press Enter to retry"
	}

	return info
}

func truncateText(s string, maxCols int) string {
	if lipgloss.Width(s) <= maxCols {
		return s
	}
	if maxCols <= 3 {
		runes := []rune(s)
		for len(runes) > 0 && lipgloss.Width(string(runes)) > maxCols {
			runes = runes[:len(runes)-1]
		}
		return string(runes)
	}
	target := maxCols - 3
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > target {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

