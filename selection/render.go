package selection

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/keyolk/tuikit/render"
)

// DefaultStyle is reverse video that survives NO_COLOR. lipgloss's default
// renderer drops every SGR under NO_COLOR, reverse included, so a style built
// from it draws no highlight at all; this one is pinned to the ANSI profile.
// NO_COLOR asks for no color, and reverse is not a color.
func DefaultStyle() lipgloss.Style {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return r.NewStyle().Reverse(true)
}

// Render draws the selection over frame (the View output) with style. It is
// pure; call it at the end of View. Rows outside the pane's visible window
// are skipped, and the selected cells lose their own colors for style's.
// A style that draws nothing (a color-only style under NO_COLOR) is replaced
// by DefaultStyle, so a selection is never invisible. Kitty image
// placeholder cells are left as they are: their foreground color is the
// image id, and restyling them would break the picture.
func (s Selection) Render(frame string, panes []Pane, style lipgloss.Style) string {
	id, a, b, ok := s.Range()
	if !ok {
		return frame
	}
	p, ok := paneByID(panes, id)
	if !ok {
		return frame
	}
	if style.Render("x") == "x" {
		style = DefaultStyle()
	}
	rows := strings.Split(frame, "\n")
	for y := p.Rect.Y; y < p.Rect.Y+p.Rect.H && y < len(rows); y++ {
		row := y - p.Rect.Y
		if p.Lines != nil {
			row += p.Offset
		}
		if row < a.Row || row > b.Row {
			continue
		}
		lo, hi := 0, p.Rect.W-1
		if row == a.Row {
			lo = a.Col
		}
		if row == b.Row {
			hi = b.Col
		}
		x := p.Rect.X + lo
		cells := ansi.Strip(ansi.Cut(rows[y], x, p.Rect.X+hi+1))
		for _, run := range textRuns(cells) {
			frame = render.Overlay(frame, style.Render(run.text), x+run.x, y)
		}
	}
	return frame
}
