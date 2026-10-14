package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

type configPane int

const (
	paneFiles configPane = iota
	paneStanzas
	paneProperties
)

type configFilesLoadedMsg struct {
	files []string
	err   error
}

type configStanzasLoadedMsg struct {
	confFile string
	stanzas  []splunk.ConfigStanza
	err      error
}

// ConfigExplorerModel provides an interactive browser for Splunk configuration files and stanzas.
type ConfigExplorerModel struct {
	theme            theme.Theme
	client           *splunk.Client
	files            []string
	fileCursor       int
	stanzas          []splunk.ConfigStanza
	stanzaCursor     int
	propScroll       int
	activePane       configPane
	filterInput      textinput.Model
	filtering        bool
	loading          bool
	loadingMsg       string
	spinner          spinner.Model
	errMsg           string
	selectedConfFile string
	width            int
	height           int
}

// NewConfigExplorer creates a new configuration explorer modal.
func NewConfigExplorer(th theme.Theme, client *splunk.Client) ConfigExplorerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)

	ti := textinput.New()
	ti.Placeholder = "Filter stanzas or properties... (/ to filter, esc to clear)"
	ti.Prompt = "🔍 "

	m := ConfigExplorerModel{
		theme:       th,
		client:      client,
		activePane:  paneFiles,
		filterInput: ti,
		spinner:     s,
		loading:     true,
		loadingMsg:  "Loading configuration files...",
	}

	return m
}

// Init loads available config files.
func (m ConfigExplorerModel) Init() tea.Cmd {
	client := m.client
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			files, err := client.GetConfigFiles(ctx)
			return configFilesLoadedMsg{files: files, err: err}
		},
	)
}

func (m ConfigExplorerModel) loadStanzasCmd(confFile string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		stanzas, err := client.GetConfigStanzas(ctx, confFile)
		return configStanzasLoadedMsg{confFile: confFile, stanzas: stanzas, err: err}
	}
}

// SetDimensions updates dimensions for the modal.
func (m *ConfigExplorerModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// Update handles navigation and filtering within the config explorer.
func (m ConfigExplorerModel) Update(msg tea.Msg) (ConfigExplorerModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil, false

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd, false
		}

	case configFilesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil, false
		}
		m.files = msg.files
		if len(m.files) > 0 {
			m.fileCursor = 0
		}
		return m, nil, false

	case configStanzasLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil, false
		}
		m.selectedConfFile = msg.confFile
		m.stanzas = msg.stanzas
		m.stanzaCursor = 0
		m.propScroll = 0
		m.activePane = paneStanzas
		return m, nil, false

	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "esc":
				m.filtering = false
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				return m, nil, false
			case "enter":
				m.filtering = false
				m.filterInput.Blur()
				return m, nil, false
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				return m, cmd, false
			}
		}

		switch msg.String() {
		case "esc", "q":
			if m.activePane == paneProperties {
				m.activePane = paneStanzas
				return m, nil, false
			}
			if m.activePane == paneStanzas {
				m.activePane = paneFiles
				return m, nil, false
			}
			return m, nil, true // close modal

		case "/":
			m.filtering = true
			m.filterInput.Focus()
			return m, nil, false

		case "tab":
			m.activePane = (m.activePane + 1) % 3
			return m, nil, false

		case "shift+tab":
			m.activePane = (m.activePane + 2) % 3
			return m, nil, false

		case "left", "h":
			if m.activePane == paneProperties {
				m.activePane = paneStanzas
			} else if m.activePane == paneStanzas {
				m.activePane = paneFiles
			}
			return m, nil, false

		case "right", "l", "enter":
			if m.activePane == paneFiles && len(m.files) > 0 {
				m.loading = true
				m.errMsg = ""
				file := m.files[m.fileCursor]
				m.loadingMsg = fmt.Sprintf("Loading %s.conf...", file)
				return m, tea.Batch(m.spinner.Tick, m.loadStanzasCmd(file)), false
			}
			if m.activePane == paneStanzas && len(m.filteredStanzas()) > 0 {
				m.activePane = paneProperties
				m.propScroll = 0
				return m, nil, false
			}

		case "up", "k":
			switch m.activePane {
			case paneFiles:
				if m.fileCursor > 0 {
					m.fileCursor--
				}
			case paneStanzas:
				if m.stanzaCursor > 0 {
					m.stanzaCursor--
				}
			case paneProperties:
				if m.propScroll > 0 {
					m.propScroll--
				}
			}
			return m, nil, false

		case "down", "j":
			switch m.activePane {
			case paneFiles:
				if m.fileCursor < len(m.files)-1 {
					m.fileCursor++
				}
			case paneStanzas:
				stanzas := m.filteredStanzas()
				if m.stanzaCursor < len(stanzas)-1 {
					m.stanzaCursor++
				}
			case paneProperties:
				m.propScroll++
			}
			return m, nil, false

		case "pgdown":
			switch m.activePane {
			case paneFiles:
				m.fileCursor += 10
				if m.fileCursor >= len(m.files) {
					m.fileCursor = len(m.files) - 1
				}
			case paneStanzas:
				stanzas := m.filteredStanzas()
				m.stanzaCursor += 10
				if m.stanzaCursor >= len(stanzas) {
					m.stanzaCursor = len(stanzas) - 1
				}
			case paneProperties:
				m.propScroll += 10
			}
			return m, nil, false

		case "pgup":
			switch m.activePane {
			case paneFiles:
				m.fileCursor -= 10
				if m.fileCursor < 0 {
					m.fileCursor = 0
				}
			case paneStanzas:
				m.stanzaCursor -= 10
				if m.stanzaCursor < 0 {
					m.stanzaCursor = 0
				}
			case paneProperties:
				m.propScroll -= 10
				if m.propScroll < 0 {
					m.propScroll = 0
				}
			}
			return m, nil, false
		}
	}

	return m, nil, false
}

func (m ConfigExplorerModel) filteredStanzas() []splunk.ConfigStanza {
	q := strings.TrimSpace(strings.ToLower(m.filterInput.Value()))
	if q == "" {
		return m.stanzas
	}
	var res []splunk.ConfigStanza
	for _, s := range m.stanzas {
		if strings.Contains(strings.ToLower(s.Name), q) {
			res = append(res, s)
			continue
		}
		for k, v := range s.Properties {
			if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
				res = append(res, s)
				break
			}
		}
	}
	return res
}

// View renders the configuration explorer as a dedicated full-screen workspace.
func (m ConfigExplorerModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	// 1. Header (1 line, full width)
	confContext := ""
	if m.selectedConfFile != "" {
		confContext = fmt.Sprintf(" ● %s.conf", m.selectedConfFile)
	}
	title := fmt.Sprintf(" ⚙ Splunk Configuration Explorer%s", confContext)
	titleStyled := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(title)
	helpText := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Tab/←/→ pane • ↑/↓ navigate • Enter select • / filter • Esc close ")
	gap := w - lipgloss.Width(titleStyled) - lipgloss.Width(helpText)
	if gap < 1 {
		gap = 1
	}
	headerLine := m.theme.Header.Width(w).MaxHeight(1).Render(titleStyled + strings.Repeat(" ", gap) + helpText)

	// 2. Filter input bar (1 line, full width)
	filterView := m.filterInput.View()
	filterLine := lipgloss.NewStyle().
		Background(m.theme.Surface).
		Width(w).
		MaxHeight(1).
		Padding(0, 1).
		Render(filterView)

	// 3. Middle 3-Column Content area
	// Header (1) + Filter (1) + Footer (1) = 3 lines
	contentH := h - 3
	if contentH < 5 {
		contentH = 5
	}
	listH := contentH - 1 // Pane header takes 1 line
	if listH < 2 {
		listH = 2
	}

	var content string
	if m.loading {
		spinView := m.spinner.View()
		content = lipgloss.NewStyle().
			Width(w).
			Height(contentH).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.Primary).
			Bold(true).
			Render(fmt.Sprintf("%s %s", spinView, m.loadingMsg))
	} else if m.errMsg != "" {
		errBox := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render(" ✖ Error: " + m.errMsg)
		content = lipgloss.NewStyle().Width(w).Height(contentH).Render(errBox)
	} else {
		sepW := 4 // " │ " divider spacing (2 dividers = 4 chars)
		availW := w - sepW
		fileColW := int(float64(availW) * 0.20)
		if fileColW < 18 {
			fileColW = 18
		}
		stanzaColW := int(float64(availW) * 0.30)
		if stanzaColW < 24 {
			stanzaColW = 24
		}
		propColW := availW - fileColW - stanzaColW
		if propColW < 24 {
			propColW = 24
		}

		// Pane 1: Files (.conf) with viewport windowing
		startFile := 0
		if m.fileCursor >= listH {
			startFile = m.fileCursor - listH + 1
		}
		endFile := startFile + listH
		if endFile > len(m.files) {
			endFile = len(m.files)
			startFile = endFile - listH
			if startFile < 0 {
				startFile = 0
			}
		}

		fileHeader := "1. Files (.conf)"
		if len(m.files) > 0 {
			fileHeader = fmt.Sprintf("1. Files (%d/%d)", m.fileCursor+1, len(m.files))
		}
		fileLines := []string{m.renderPaneHeader(fileHeader, m.activePane == paneFiles, fileColW)}
		for i := startFile; i < endFile; i++ {
			item := m.files[i] + ".conf"
			if i == m.fileCursor && m.activePane == paneFiles {
				fileLines = append(fileLines, m.theme.TableSelected.Width(fileColW).Render("▶ "+truncate(item, fileColW-3)))
			} else if i == m.fileCursor {
				fileLines = append(fileLines, lipgloss.NewStyle().Foreground(m.theme.Highlight).Width(fileColW).Render("● "+truncate(item, fileColW-3)))
			} else {
				fileLines = append(fileLines, lipgloss.NewStyle().Foreground(m.theme.Text).Width(fileColW).Render("  "+truncate(item, fileColW-3)))
			}
		}
		for len(fileLines) < listH+1 {
			fileLines = append(fileLines, strings.Repeat(" ", fileColW))
		}
		fileBox := lipgloss.NewStyle().Width(fileColW).MaxHeight(contentH).Render(strings.Join(fileLines, "\n"))

		// Pane 2: Stanzas with viewport windowing
		stanzas := m.filteredStanzas()
		stanzaHeader := "2. Stanzas"
		if len(stanzas) > 0 {
			stanzaHeader = fmt.Sprintf("2. Stanzas (%d/%d)", m.stanzaCursor+1, len(stanzas))
		}
		stanzaLines := []string{m.renderPaneHeader(stanzaHeader, m.activePane == paneStanzas, stanzaColW)}
		if len(stanzas) == 0 {
			emptyText := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("  No stanzas found")
			stanzaLines = append(stanzaLines, emptyText)
		} else {
			startStanza := 0
			if m.stanzaCursor >= listH {
				startStanza = m.stanzaCursor - listH + 1
			}
			endStanza := startStanza + listH
			if endStanza > len(stanzas) {
				endStanza = len(stanzas)
				startStanza = endStanza - listH
				if startStanza < 0 {
					startStanza = 0
				}
			}

			for i := startStanza; i < endStanza; i++ {
				s := stanzas[i]
				name := s.Name
				if !strings.HasPrefix(name, "[") {
					name = "[" + name + "]"
				}
				name = truncate(name, stanzaColW-4)
				if i == m.stanzaCursor && m.activePane == paneStanzas {
					stanzaLines = append(stanzaLines, m.theme.TableSelected.Width(stanzaColW).Render("▶ "+name))
				} else if i == m.stanzaCursor {
					stanzaLines = append(stanzaLines, lipgloss.NewStyle().Foreground(m.theme.Highlight).Width(stanzaColW).Render("● "+name))
				} else {
					stanzaLines = append(stanzaLines, lipgloss.NewStyle().Foreground(m.theme.Text).Width(stanzaColW).Render("  "+name))
				}
			}
		}
		for len(stanzaLines) < listH+1 {
			stanzaLines = append(stanzaLines, strings.Repeat(" ", stanzaColW))
		}
		stanzaBox := lipgloss.NewStyle().Width(stanzaColW).MaxHeight(contentH).Render(strings.Join(stanzaLines, "\n"))

		// Pane 3: Properties with viewport windowing
		var propKeys []string
		var selectedStanza splunk.ConfigStanza
		if len(stanzas) > 0 && m.stanzaCursor < len(stanzas) {
			selectedStanza = stanzas[m.stanzaCursor]
			for k := range selectedStanza.Properties {
				propKeys = append(propKeys, k)
			}
			sort.Strings(propKeys)
		}

		propHeader := "3. Properties"
		if len(propKeys) > 0 {
			propHeader = fmt.Sprintf("3. Properties (%d/%d)", m.propScroll+1, len(propKeys))
		}
		propLines := []string{m.renderPaneHeader(propHeader, m.activePane == paneProperties, propColW)}
		if len(propKeys) == 0 {
			emptyProps := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("  Select a stanza")
			propLines = append(propLines, emptyProps)
		} else {
			maxPropScroll := len(propKeys) - listH
			if maxPropScroll < 0 {
				maxPropScroll = 0
			}
			if m.propScroll > maxPropScroll {
				m.propScroll = maxPropScroll
			}
			startProp := m.propScroll
			endProp := startProp + listH
			if endProp > len(propKeys) {
				endProp = len(propKeys)
			}

			keyWidth := int(float64(propColW) * 0.35)
			if keyWidth < 12 {
				keyWidth = 12
			}
			valWidth := propColW - keyWidth - 3
			if valWidth < 12 {
				valWidth = 12
			}

			for i := startProp; i < endProp; i++ {
				k := propKeys[i]
				v := selectedStanza.Properties[k]
				keyStyled := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Width(keyWidth).Render(truncate(k, keyWidth-1))
				valStyled := lipgloss.NewStyle().Foreground(m.theme.Text).Width(valWidth).Render(truncate(v, valWidth-1))
				propLines = append(propLines, lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " = ", valStyled))
			}
		}
		for len(propLines) < listH+1 {
			propLines = append(propLines, strings.Repeat(" ", propColW))
		}
		propBox := lipgloss.NewStyle().Width(propColW).MaxHeight(contentH).Render(strings.Join(propLines, "\n"))

		divider := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render("│")
		content = lipgloss.JoinHorizontal(lipgloss.Top, fileBox, " ", divider, " ", stanzaBox, " ", divider, " ", propBox)
	}

	// 4. Footer status bar (1 line, full width)
	paneNames := map[configPane]string{
		paneFiles:      "Active: 1. Files (.conf)",
		paneStanzas:    "Active: 2. Stanzas",
		paneProperties: "Active: 3. Properties",
	}
	footerLeft := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render(" " + paneNames[m.activePane])
	footerRight := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("PgUp/PgDn: scroll • Esc: back/close ")
	footerGap := w - lipgloss.Width(footerLeft) - lipgloss.Width(footerRight)
	if footerGap < 1 {
		footerGap = 1
	}
	footerLine := m.theme.StatusBar.Width(w).MaxHeight(1).Render(footerLeft + strings.Repeat(" ", footerGap) + footerRight)

	fullView := lipgloss.JoinVertical(lipgloss.Left, headerLine, filterLine, content, footerLine)
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(fullView)
}

func (m ConfigExplorerModel) renderPaneHeader(title string, isActive bool, width int) string {
	style := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Bold(true)
	if isActive {
		style = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Underline(true)
	}
	return style.Width(width).Render(title)
}
