package kitty

import (
	"math"

	tea "github.com/charmbracelet/bubbletea"
)

// CellSize is one terminal cell in pixels.
type CellSize struct{ W, H int }

// DefaultCellSize is used until the terminal says otherwise: cells about
// twice as tall as wide, the assumption ccx's FitSize made.
func DefaultCellSize() CellSize { return CellSize{W: 10, H: 20} }

// OrDefault returns c, or DefaultCellSize when either side is not positive.
func (c CellSize) OrDefault() CellSize {
	if c.W <= 0 || c.H <= 0 {
		return DefaultCellSize()
	}
	return c
}

// Fit returns the largest cols×rows box, within maxCols×maxRows, that shows
// an imgW×imgH pixel image without distorting it, given the cell's pixel
// size (a non-positive cell size means DefaultCellSize). The image is scaled
// up as well as down: the box is what the terminal stretches it to. Each side
// is at least 1 when the limits allow. An unknown image size (≤ 0) gets the
// whole box, as in ccx.
func Fit(imgW, imgH, maxCols, maxRows, cellW, cellH int) (cols, rows int) {
	maxCols, maxRows = max(maxCols, 0), max(maxRows, 0)
	if imgW <= 0 || imgH <= 0 || maxCols == 0 || maxRows == 0 {
		return maxCols, maxRows
	}
	cell := CellSize{cellW, cellH}.OrDefault()
	// The image's size in cells at 1:1, then one scale for both axes.
	wc := float64(imgW) / float64(cell.W)
	hc := float64(imgH) / float64(cell.H)
	scale := math.Min(float64(maxCols)/wc, float64(maxRows)/hc)
	// The epsilon keeps 39.999… from flooring to 39 when the box fits exactly.
	cols = int(math.Floor(wc*scale + 1e-9))
	rows = int(math.Floor(hc*scale + 1e-9))
	return min(max(cols, 1), maxCols), min(max(rows, 1), maxRows)
}

// CellSizeMsg reports CellSizeCmd. On error Size is DefaultCellSize.
type CellSizeMsg struct {
	Size CellSize
	Err  error
}

// CellSizeCmd reads the cell size from the terminal's window size in pixels
// (TIOCGWINSZ) on fd, usually os.Stdout.Fd(). It needs no reply on stdin, so
// it works inside tmux, which reports the outer terminal's pixels since 3.3.
// Where pixels are not reported it fails, and the app keeps the default or
// asks with CellSizeQuery.
func CellSizeCmd(fd uintptr) tea.Cmd {
	return func() tea.Msg {
		c, err := cellSizeFromFD(fd)
		if err != nil {
			return CellSizeMsg{Size: DefaultCellSize(), Err: err}
		}
		return CellSizeMsg{Size: c}
	}
}
