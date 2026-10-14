package main

import (
	"image"
	"image/color"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/kitty"
	"github.com/keyolk/tuikit/toast"
)

// picture is the kitty hook: a generated gradient under the detail text,
// drawn with Unicode placeholders, or alt text where the terminal has no
// graphics.
type picture struct {
	caps kitty.Caps // detected once in main
	out  kitty.Out  // /dev/tty, beside the frame
	cell kitty.CellSize
	img  kitty.Image
}

const picW, picH = 64, 32 // pixels

func newPicture(caps kitty.Caps, out kitty.Out) picture {
	return picture{caps: caps, out: out, img: kitty.Image{
		ID: 1,
		// The Key lets Cmds see an unchanged picture without comparing pixels.
		Source: kitty.Source{Key: "gradient", Img: gradient(picW, picH)},
		Alt:    "그림 자리: 이 터미널은 kitty 그래픽을 지원하지 않습니다",
	}}
}

// gradient stands in for a real image, so the demo needs no file.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 160, A: 255})
		}
	}
	return img
}

// initCmd asks for the cell size and lets tmux pass graphics from a hidden
// pane; both run off the UI loop.
func (p picture) initCmd(fd uintptr, tmux kitty.TmuxQuerier) tea.Cmd {
	if !p.caps.Supported {
		return nil
	}
	return tea.Batch(kitty.CellSizeCmd(fd), kitty.EnsurePassthroughCmd(p.caps, tmux))
}

// resize fits the picture into a w×h box and returns what the terminal
// needs: a transmit when the box changed, nothing when it did not.
func (p picture) resize(w, h int) (picture, tea.Cmd) {
	next := p.img
	next.Cols, next.Rows = kitty.Fit(picW, picH, w, h, p.cell.W, p.cell.H)
	if !next.Shown(p.caps) {
		next.Cols, next.Rows = w, h // the alt text gets the whole box
	}
	cmd := next.Cmds(p.img, p.caps, p.out, p.cell)
	p.img = next
	return p, cmd
}

// pictureBox is the box under the detail text; View and Update both use it
// (E9). It ignores toasts, so one appearing does not resend the picture; View
// cuts whole rows if the body gets shorter than the box.
func (m model) pictureBox() (w, h int) {
	return m.w - min(30, m.w/2) - 1, min(8, max(0, (m.h-2)/2))
}

// updatePicture handles the kitty messages; the bool says it took msg.
func (m model) updatePicture(msg tea.Msg) (model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case kitty.CellSizeMsg:
		if msg.Err != nil {
			m.logf("cell size: %v (using %dx%d)", msg.Err, msg.Size.W, msg.Size.H)
		}
		m.pic.cell = msg.Size
		// Same box, new pixels per cell: Fit may pick a new size.
		var cmd tea.Cmd
		m.pic, cmd = m.pic.resize(m.pictureBox())
		return m, cmd, true
	case kitty.PassthroughMsg:
		if msg.Err != nil {
			m.logf("tmux allow-passthrough: %v", msg.Err)
		}
		m.pic.caps = m.pic.caps.WithPassthrough(msg)
		return m, nil, true
	case kitty.Wrote:
		if msg.Err != nil {
			m.toasts = toast.Add(m.toasts, toast.Toast{Source: "picture", Text: "picture: " + msg.Err.Error(), Level: toast.Error})
		}
		return m, nil, true
	}
	return m, nil, false
}
