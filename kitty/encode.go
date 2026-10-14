package kitty

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
)

// ChunkSize is the largest base64 payload in one escape. The protocol caps
// a chunk at 4096 bytes; every chunk but the last is exactly that long.
const ChunkSize = 4096

// Format is the pixel format of transmitted data (the f key).
type Format int

const (
	PNG  Format = 100 // the zero TransmitOpts means PNG
	RGBA Format = 32  // raw 8-bit RGBA; needs Width and Height
	RGB  Format = 24  // raw 8-bit RGB; needs Width and Height
)

// TransmitOpts describe the data. The zero value is PNG.
type TransmitOpts struct {
	Format Format
	// Width and Height are the pixel size, required for raw formats.
	Width, Height int
}

// Wrap wraps seq in tmux's DCS passthrough when c needs it: every ESC inside
// is doubled and the whole is framed by ESC P tmux; ... ESC \.
func (c Caps) Wrap(seq string) string {
	if !c.NeedsPassthrough || seq == "" {
		return seq
	}
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// apc frames one graphics command.
func apc(keys, payload string) string {
	if payload == "" {
		return "\x1b_G" + keys + "\x1b\\"
	}
	return "\x1b_G" + keys + ";" + payload + "\x1b\\"
}

func (o TransmitOpts) keys(medium string, id uint32) string {
	f := o.Format
	if f == 0 {
		f = PNG
	}
	k := fmt.Sprintf("a=t,t=%s,f=%d,i=%d,q=2", medium, f, id)
	if o.Width > 0 && o.Height > 0 {
		k += fmt.Sprintf(",s=%d,v=%d", o.Width, o.Height)
	}
	return k
}

// Transmit uploads data (PNG unless o says otherwise) under id without
// displaying it (a=t), in ChunkSize pieces with m=1 on all but the last.
// Each chunk is wrapped on its own: one passthrough around a large image
// overruns tmux's per-sequence limit and the image never arrives (ccproxy).
// q=2 suppresses replies, which a TUI would read as keys. "" when
// unsupported, id is 0 or data is empty.
func (c Caps) Transmit(id uint32, data []byte, o TransmitOpts) string {
	if !c.Supported || id == 0 || len(data) == 0 {
		return ""
	}
	enc := base64.StdEncoding.EncodeToString(data)
	first := o.keys("d", id)
	var b strings.Builder
	for i := 0; i < len(enc); i += ChunkSize {
		end := min(i+ChunkSize, len(enc))
		more := 0
		if end < len(enc) {
			more = 1
		}
		k := fmt.Sprintf("m=%d,q=2", more)
		if i == 0 {
			k = first + fmt.Sprintf(",m=%d", more)
		}
		b.WriteString(c.Wrap(apc(k, enc[i:end])))
	}
	return b.String()
}

// TransmitImage encodes img as PNG and transmits it. The pixels are sent as
// they are: shrink a large image first (EncodeForGrid) so the payload stays
// small.
func (c Caps) TransmitImage(id uint32, img image.Image) (string, error) {
	if !c.Supported || id == 0 || img == nil {
		return "", nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("kitty: encode image %d: %w", id, err)
	}
	return c.Transmit(id, buf.Bytes(), TransmitOpts{}), nil
}

// TransmitFile tells the terminal to read the image from path (t=f). The
// terminal reads the file, so it must run on the same machine (ccx).
func (c Caps) TransmitFile(id uint32, path string, o TransmitOpts) string {
	if !c.Supported || id == 0 || path == "" {
		return ""
	}
	return c.Wrap(apc(o.keys("f", id), base64.StdEncoding.EncodeToString([]byte(path))))
}

// TransmitShared tells the terminal to read size bytes from the POSIX
// shared-memory object name (t=s). The terminal unlinks it after reading.
func (c Caps) TransmitShared(id uint32, name string, size int, o TransmitOpts) string {
	if !c.Supported || id == 0 || name == "" || size <= 0 {
		return ""
	}
	return c.Wrap(apc(o.keys("s", id)+fmt.Sprintf(",S=%d", size), base64.StdEncoding.EncodeToString([]byte(name))))
}

// PlaceOpts position a placement.
type PlaceOpts struct {
	// Z is the z-index. Negative draws under text: z=-1 keeps the pane's
	// glyphs readable over the image (how TWeb shows a page in a pane).
	Z int
	// Virtual makes a placement for Unicode placeholders (U=1). Its size is
	// the cell grid the placeholder block covers.
	Virtual bool
	// MoveTo places at Row, Col (0-based, relative to the pane) instead of
	// at the cursor; the cursor is saved and restored around it. Inside tmux
	// the move goes through the same passthrough, so PaneTop and PaneLeft
	// (from QueryPane) turn it into terminal coordinates (ccx).
	MoveTo            bool
	Row, Col          int
	PaneTop, PaneLeft int
}

// Place shows image id over cols×rows cells. placement (p) names this
// placement so a later Place with the same pair replaces it rather than
// adding a second; 0 lets the terminal pick. C=1 keeps the cursor still.
func (c Caps) Place(id, placement uint32, cols, rows int, o PlaceOpts) string {
	if !c.Supported || id == 0 || cols < 1 || rows < 1 {
		return ""
	}
	k := fmt.Sprintf("a=p,i=%d", id)
	if placement != 0 {
		k += fmt.Sprintf(",p=%d", placement)
	}
	k += fmt.Sprintf(",c=%d,r=%d", cols, rows)
	if o.Virtual {
		k += ",U=1"
	}
	if o.Z != 0 {
		k += fmt.Sprintf(",z=%d", o.Z)
	}
	k += ",C=1,q=2"
	seq := apc(k, "")
	if o.MoveTo {
		// CUP is 1-based; save (ESC 7) and restore (ESC 8) the cursor so the
		// app's own cursor position is untouched.
		seq = fmt.Sprintf("\x1b7\x1b[%d;%dH", o.Row+o.PaneTop+1, o.Col+o.PaneLeft+1) + seq + "\x1b8"
	}
	return c.Wrap(seq)
}

// Delete removes image id, its placements and its data (d=I).
func (c Caps) Delete(id uint32) string {
	if !c.Supported || id == 0 {
		return ""
	}
	return c.Wrap(apc(fmt.Sprintf("a=d,d=I,i=%d,q=2", id), ""))
}

// DeleteAll removes every image and placement and frees their data (d=A).
func (c Caps) DeleteAll() string {
	if !c.Supported {
		return ""
	}
	return c.Wrap(apc("a=d,d=A,q=2", ""))
}
