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

// View renders the configuration explorer modal.
func (m ConfigExplorerModel) View() string {
	boxW := m.width - 6
	if boxW < 60 {
		boxW = 60
	}
	boxH := m.height - 4
	if boxH < 18 {
		boxH = 18
	}

	innerW := boxW - 4
	innerH := boxH - 2

	// Header
	title := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render("⚙ Splunk Configuration Explorer")
	helpText := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("tab/←/→: switch pane • enter: select • /: filter • esc: back/close")
	gap := innerW - lipgloss.Width(title) - lipgloss.Width(helpText)
	if gap < 2 {
		gap = 2
	}
	headerLine := title + strings.Repeat(" ", gap) + helpText

	// Filter Bar
	filterBar := lipgloss.NewStyle().Width(innerW).Render(m.filterInput.View())

	contentH := innerH - 4
	if contentH < 8 {
		contentH = 8
	}

	var content string
	if m.loading {
		spinView := m.spinner.View()
		content = lipgloss.NewStyle().
			Width(innerW).
			Height(contentH).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.Primary).
			Bold(true).
			Render(fmt.Sprintf("%s %s", spinView, m.loadingMsg))
	} else if m.errMsg != "" {
		errBox := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("✖ Error: " + m.errMsg)
		content = lipgloss.NewStyle().Width(innerW).Height(contentH).Render(errBox)
	} else {
		// 3-Pane Layout: Conf Files (20 cols) | Stanzas (30 cols) | Key-Value Settings (rest)
		fileColW := 22
		stanzaColW := 32
		propColW := innerW - fileColW - stanzaColW - 4
		if propColW < 20 {
			propColW = 20
		}

		// Pane 1: Files
		fileLines := []string{m.renderPaneHeader("1. Files (.conf)", m.activePane == paneFiles, fileColW)}
		for i, f := range m.files {
			item := f + ".conf"
			if i == m.fileCursor && m.activePane == paneFiles {
				fileLines = append(fileLines, m.theme.TableSelected.Width(fileColW).Render("▶ "+item))
			} else if i == m.fileCursor {
				fileLines = append(fileLines, lipgloss.NewStyle().Foreground(m.theme.Highlight).Width(fileColW).Render("● "+item))
			} else {
				fileLines = append(fileLines, lipgloss.NewStyle().Foreground(m.theme.Text).Width(fileColW).Render("  "+item))
			}
		}
		fileBox := lipgloss.NewStyle().Width(fileColW).Height(contentH).Render(strings.Join(fileLines, "\n"))

		// Pane 2: Stanzas
		stanzas := m.filteredStanzas()
		confLabel := m.selectedConfFile
		if confLabel == "" {
			confLabel = "select file"
		}
		stanzaLines := []string{m.renderPaneHeader(fmt.Sprintf("2. Stanzas (%d)", len(stanzas)), m.activePane == paneStanzas, stanzaColW)}
		if len(stanzas) == 0 {
			emptyText := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("  No stanzas loaded")
			stanzaLines = append(stanzaLines, emptyText)
		} else {
			for i, s := range stanzas {
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
		stanzaBox := lipgloss.NewStyle().Width(stanzaColW).Height(contentH).Render(strings.Join(stanzaLines, "\n"))

		// Pane 3: Properties
		propLines := []string{m.renderPaneHeader("3. Properties", m.activePane == paneProperties, propColW)}
		if len(stanzas) > 0 && m.stanzaCursor < len(stanzas) {
			selectedStanza := stanzas[m.stanzaCursor]
			var propKeys []string
			for k := range selectedStanza.Properties {
				propKeys = append(propKeys, k)
			}
			sort.Strings(propKeys)

			for _, k := range propKeys {
				v := selectedStanza.Properties[k]
				keyStyled := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Width(propColW / 2).Render(truncate(k, propColW/2-2))
				valStyled := lipgloss.NewStyle().Foreground(m.theme.Text).Width(propColW / 2).Render(truncate(v, propColW/2-2))
				propLines = append(propLines, lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " = ", valStyled))
			}
		} else {
			emptyProps := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("  Select a stanza to view properties")
			propLines = append(propLines, emptyProps)
		}
		propBox := lipgloss.NewStyle().Width(propColW).Height(contentH).Render(strings.Join(propLines, "\n"))

		divider := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render("│")
		content = lipgloss.JoinHorizontal(lipgloss.Top, fileBox, " ", divider, " ", stanzaBox, " ", divider, " ", propBox)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, headerLine, "", filterBar, "", content)
	return m.theme.FocusedCard.Width(boxW).Height(boxH).Render(body)
}

func (m ConfigExplorerModel) renderPaneHeader(title string, isActive bool, width int) string {
	style := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Bold(true)
	if isActive {
		style = lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Underline(true)
	}
	return style.Width(width).Render(title)
}
