package kitty

import (
	"bytes"
	"fmt"
	"image"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/render"
)

// Source is an image's pixels. The first non-empty of Img, Data and Path is
// used. Img and Data are shared by copies of the value and must not be
// changed after they are set: replace them instead.
type Source struct {
	// Key names the pixels: two Sources with the same non-empty Key are the
	// same image, so Cmds does not send it again. Without a Key, Data and
	// Path are compared, and an Img is always sent again.
	Key string
	// Img is decoded pixels; it is shrunk to the box and sent as PNG.
	Img image.Image
	// Data is encoded bytes. PNG is sent as it is; other formats are
	// decoded and shrunk like Img.
	Data []byte
	// Path is a PNG file the terminal reads itself (same machine only).
	Path string
}

func (s Source) empty() bool { return s.Img == nil && len(s.Data) == 0 && s.Path == "" }

func (s Source) same(o Source) bool {
	if s.Key != "" || o.Key != "" {
		return s.Key == o.Key
	}
	if s.Img != nil || o.Img != nil {
		return false // an image.Image cannot be compared cheaply
	}
	if len(s.Data) != len(o.Data) {
		return false
	}
	if len(s.Data) > 0 && &s.Data[0] != &o.Data[0] && !bytes.Equal(s.Data, o.Data) {
		return false
	}
	return s.Path == o.Path
}

// placementID is the one virtual placement each Image keeps, so placing
// again replaces it instead of piling up placements of the same image.
const placementID = 1

// Image shows a picture in a Cols×Rows box of cells, or Alt where the
// terminal cannot. It is a value: keep it in the model, change fields, and
// pass the old value to Cmds.
//
//	next := m.img
//	next.Cols, next.Rows = kitty.Fit(w, h, paneW, paneH, m.cell.W, m.cell.H)
//	cmd := next.Cmds(m.img, m.caps, m.out, m.cell)
//	m.img = next
type Image struct {
	// ID is the terminal's image id, unique among the app's images.
	// Keep it in 1..255: larger ids need 24-bit color through tmux.
	ID     uint32
	Source Source
	// Cols and Rows are the box in cells. View always fills it. The image
	// itself uses at most SafeRows rows.
	Cols, Rows int
	// Alt is drawn, word-wrapped, when the image cannot be.
	Alt string
}

// Shown reports whether View draws the image rather than Alt.
func (im Image) Shown(c Caps) bool {
	return c.Supported && im.ID != 0 && !im.Source.empty() && im.Cols > 0 && im.Rows > 0
}

// imageRows is how many rows the picture covers.
func (im Image) imageRows() int { return min(im.Rows, SafeRows) }

// View returns exactly Rows lines of exactly Cols cells: placeholder cells
// when Shown, otherwise Alt wrapped and cut to the box. Do not truncate or
// restyle the lines: keep them whole and frame them from outside.
func (im Image) View(c Caps) string {
	if im.Cols <= 0 || im.Rows <= 0 {
		return ""
	}
	var lines []string
	if im.Shown(c) {
		lines = Placeholder(im.ID, im.Cols, im.imageRows())
	} else {
		lines = strings.Split(ansi.Wrap(im.Alt, im.Cols, ""), "\n")
		lines = lines[:min(len(lines), im.Rows)]
		for i, l := range lines {
			lines[i] = render.Pad(l, im.Cols)
		}
	}
	blank := strings.Repeat(" ", im.Cols)
	for len(lines) < im.Rows {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

// Cmds returns the command that brings the terminal from prev to im: send
// the image when it appears or its pixels, id or size change; delete it when
// it is hidden or its id changes. nil when nothing changed or the terminal
// cannot show images. Encoding runs inside the command. The result is a
// Wrote with any error.
func (im Image) Cmds(prev Image, c Caps, out Out, cell CellSize) tea.Cmd {
	if !c.Supported {
		return nil
	}
	was, is := prev.Shown(c), im.Shown(c)
	if was == is && (!is || prev.ID == im.ID && prev.Cols == im.Cols &&
		prev.imageRows() == im.imageRows() && prev.Source.same(im.Source)) {
		return nil
	}
	var del string
	if was && (!is || prev.ID != im.ID) {
		del = c.Delete(prev.ID)
	}
	if !is {
		return func() tea.Msg { return Wrote{ID: prev.ID, Err: out.Write(del)} }
	}
	return func() tea.Msg {
		seq, err := im.payload(c, cell)
		if err != nil {
			// The old image still goes: it no longer matches the cells.
			if werr := out.Write(del); werr != nil {
				err = fmt.Errorf("%w; %w", err, werr)
			}
			return Wrote{ID: im.ID, Err: err}
		}
		return Wrote{ID: im.ID, Err: out.Write(del + seq)}
	}
}

// payload is the transmit plus the virtual placement the cells point at.
func (im Image) payload(c Caps, cell CellSize) (string, error) {
	var seq string
	s := im.Source
	switch {
	case s.Img != nil:
		data, err := EncodeForGrid(s.Img, im.Cols, im.imageRows(), cell)
		if err != nil {
			return "", err
		}
		seq = c.Transmit(im.ID, data, TransmitOpts{})
	case len(s.Data) > 0:
		data := s.Data
		if !IsPNG(data) {
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return "", fmt.Errorf("kitty: decode image %d: %w", im.ID, err)
			}
			if data, err = EncodeForGrid(img, im.Cols, im.imageRows(), cell); err != nil {
				return "", err
			}
		}
		seq = c.Transmit(im.ID, data, TransmitOpts{})
	default:
		seq = c.TransmitFile(im.ID, s.Path, TransmitOpts{})
	}
	return seq + im.place(c), nil
}

func (im Image) place(c Caps) string {
	return c.Place(im.ID, placementID, im.Cols, im.imageRows(), PlaceOpts{Virtual: true})
}

// HideCmd deletes the image from the terminal, for an app that stops
// drawing it without passing through Cmds (leaving the screen, quitting).
func (im Image) HideCmd(c Caps, out Out) tea.Cmd {
	if !im.Shown(c) {
		return nil
	}
	seq := c.Delete(im.ID)
	return func() tea.Msg { return Wrote{ID: im.ID, Err: out.Write(seq)} }
}

// RebindCmd places the image over its cells again without resending it.
// ccproxy needed this after full repaints (resize, clear) left the image
// drawn at stale positions; it is cheap enough to send on every resize.
func (im Image) RebindCmd(c Caps, out Out) tea.Cmd {
	if !im.Shown(c) {
		return nil
	}
	seq := im.place(c)
	return func() tea.Msg { return Wrote{ID: im.ID, Err: out.Write(seq)} }
}
