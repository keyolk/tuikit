package kitty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// ErrNoOutput is what the zero Out reports instead of dropping a write.
var ErrNoOutput = errors.New("kitty: no output (zero Out)")

// Out writes escape sequences beside the frame, never through View: a
// transmit is hundreds of kilobytes of base64 that Bubble Tea would diff as
// text and corrupt (ccproxy).
//
// Out is a handle, like *os.File: copies share one writer and one lock, on
// purpose. Two commands writing at once otherwise interleave their bytes and
// raw base64 prints on screen (ccproxy). Each Write is one call to the
// underlying writer, so a frame flush cannot split a sequence.
type Out struct {
	w  io.Writer
	mu *sync.Mutex
}

// NewOut writes to w.
func NewOut(w io.Writer) Out { return Out{w: w, mu: &sync.Mutex{}} }

// OpenTTY opens /dev/tty for writing. The app closes it after the program
// exits.
func OpenTTY() (Out, io.Closer, error) {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return Out{}, nil, fmt.Errorf("kitty: open tty: %w", err)
	}
	return NewOut(f), f, nil
}

// Write sends seq in one write. An empty seq writes nothing.
func (o Out) Write(seq string) error {
	if seq == "" {
		return nil
	}
	if o.w == nil {
		return ErrNoOutput
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := io.WriteString(o.w, seq); err != nil {
		return fmt.Errorf("kitty: write: %w", err)
	}
	return nil
}

// Wrote reports a write made by a command. ID is the image it was for, 0
// for a plain WriteCmd.
type Wrote struct {
	ID  uint32
	Err error
}

// WriteCmd writes seq off the UI loop; nil when seq is empty.
func (o Out) WriteCmd(seq string) tea.Cmd {
	if seq == "" {
		return nil
	}
	return func() tea.Msg { return Wrote{Err: o.Write(seq)} }
}
