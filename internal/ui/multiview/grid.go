package multiview

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// GridRenderer arranges instance cards into a 2-per-row responsive grid with viewport scrolling.
type GridRenderer struct {
	scrollOffset int // row offset in the grid
}

// RenderGrid renders cards in pairs of 2 per row, with viewport scrolling.
func (g *GridRenderer) RenderGrid(cards []InstanceCardModel, activeIdx, width, availHeight int) string {
	if len(cards) == 0 {
		return ""
	}

	// 2 columns
	colW := (width - 6) / 2
	if colW < 30 {
		colW = 30
	}

	totalPairs := (len(cards) + 1) / 2
	if totalPairs <= 0 {
		totalPairs = 1
	}

	// Dynamically scale card height so cards fill available container height
	cardH := 12
	if totalPairs == 1 {
		cardH = availHeight
		if cardH > 24 {
			cardH = 24
		}
		if cardH < 8 {
			cardH = 8
		}
	} else {
		cardH = availHeight / 2
		if cardH < 8 {
			cardH = 8
		}
		if cardH > 18 {
			cardH = 18
		}
	}

	maxVisPairs := availHeight / cardH
	if maxVisPairs < 1 {
		maxVisPairs = 1
	}

	// Active pair row
	activePair := activeIdx / 2
	if activePair >= g.scrollOffset+maxVisPairs {
		g.scrollOffset = activePair - maxVisPairs + 1
	}
	if activePair < g.scrollOffset {
		g.scrollOffset = activePair
	}
	if g.scrollOffset > totalPairs-maxVisPairs {
		g.scrollOffset = totalPairs - maxVisPairs
	}
	if g.scrollOffset < 0 {
		g.scrollOffset = 0
	}

	startPair := g.scrollOffset
	endPair := startPair + maxVisPairs
	if endPair > totalPairs {
		endPair = totalPairs
	}

	var rowViews []string
	for p := startPair; p < endPair; p++ {
		idx1 := p * 2
		idx2 := p*2 + 1

		cards[idx1].SetDimensions(colW, cardH)
		cards[idx1].SetFocus(idx1 == activeIdx)
		v1 := cards[idx1].View()

		var v2 string
		if idx2 < len(cards) {
			cards[idx2].SetDimensions(colW, cardH)
			cards[idx2].SetFocus(idx2 == activeIdx)
			v2 = cards[idx2].View()
		} else {
			// Empty placeholder for odd card count
			v2 = strings.Repeat(" ", colW)
		}

		pairRow := lipgloss.JoinHorizontal(lipgloss.Top, v1, "  ", v2)
		rowViews = append(rowViews, pairRow)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rowViews...)
}
