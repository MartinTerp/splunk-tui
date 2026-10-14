// Package theme defines visual styling tokens and styles for splunk-tui.
package theme

import "github.com/charmbracelet/lipgloss"

// Theme defines the visual styling tokens and colors.
type Theme struct {
	Primary       lipgloss.Color
	Secondary     lipgloss.Color
	Background    lipgloss.Color
	Surface       lipgloss.Color
	SurfaceBorder lipgloss.Color
	Text          lipgloss.Color
	SubtleText    lipgloss.Color
	Highlight     lipgloss.Color
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	ChartPalette  []lipgloss.Color

	// Styles
	App           lipgloss.Style
	Header        lipgloss.Style
	Title         lipgloss.Style
	Card          lipgloss.Style
	FocusedCard   lipgloss.Style
	Button        lipgloss.Style
	ActiveButton  lipgloss.Style
	InputBox      lipgloss.Style
	StatusBar     lipgloss.Style
	HelpKey       lipgloss.Style
	HelpDesc      lipgloss.Style
	TableRow      lipgloss.Style
	TableRowAlt   lipgloss.Style
	TableSelected lipgloss.Style
	TableHeader   lipgloss.Style
	SidebarItem   lipgloss.Style
	SidebarActive lipgloss.Style
}

// DefaultTheme returns the Splunk-themed modern dark palette.
func DefaultTheme() Theme {
	t := Theme{
		Primary:       lipgloss.Color("#65A637"), // Splunk signature green
		Secondary:     lipgloss.Color("#2E7D32"),
		Background:    lipgloss.Color("#121417"),
		Surface:       lipgloss.Color("#1B1F24"),
		SurfaceBorder: lipgloss.Color("#2E3440"),
		Text:          lipgloss.Color("#E5E9F0"),
		SubtleText:    lipgloss.Color("#8892B0"),
		Highlight:     lipgloss.Color("#56B6C2"),
		Success:       lipgloss.Color("#98C379"),
		Warning:       lipgloss.Color("#E5C07B"),
		Error:         lipgloss.Color("#E06C75"),
		ChartPalette: []lipgloss.Color{
			lipgloss.Color("#65A637"), // Green
			lipgloss.Color("#56B6C2"), // Cyan
			lipgloss.Color("#E5C07B"), // Amber / Yellow
			lipgloss.Color("#C678DD"), // Purple / Magenta
			lipgloss.Color("#E06C75"), // Coral / Red
			lipgloss.Color("#61AFEF"), // Blue
			lipgloss.Color("#D19A66"), // Orange
		},
	}

	t.App = lipgloss.NewStyle().
		Background(t.Background).
		Foreground(t.Text)

	t.Header = lipgloss.NewStyle().
		Background(lipgloss.Color("#192119")).
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1)

	t.Title = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.Card = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.SurfaceBorder).
		Padding(0, 1)

	t.FocusedCard = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Padding(0, 1)

	t.Button = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(t.SurfaceBorder).
		Padding(0, 1)

	t.ActiveButton = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(t.Primary).
		Bold(true).
		Padding(0, 1)

	t.InputBox = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(t.SurfaceBorder).
		Padding(0, 1)

	t.StatusBar = lipgloss.NewStyle().
		Background(lipgloss.Color("#1A1D23")).
		Foreground(t.SubtleText).
		Padding(0, 1)

	t.HelpKey = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.HelpDesc = lipgloss.NewStyle().
		Foreground(t.SubtleText)

	t.TableHeader = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.TableRow = lipgloss.NewStyle().
		Foreground(t.Text)

	t.TableRowAlt = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(lipgloss.Color("#16191E"))

	t.TableSelected = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#1F2D1F")).
		Bold(true)

	t.SidebarItem = lipgloss.NewStyle().
		Foreground(t.Text)

	t.SidebarActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#1F2D1F")).
		Bold(true)

	return t
}
