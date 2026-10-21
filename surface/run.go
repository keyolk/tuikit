package surface

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/theme"
)

// RunOptions configure Run.
type RunOptions struct {
	Options
	// Keyboard and Screen are the terminal. When the agent stream is stdin
	// and the events go to stdout, both must be /dev/tty: the TUI cannot
	// read keys from the pipe the agent writes, nor draw on the pipe it
	// reads. Nil means os.Stdin and os.Stdout (socket mode).
	Keyboard io.Reader
	Screen   io.Writer
	// QueueSize bounds events waiting to be written (default 256). A full
	// queue drops the event and says so, rather than freezing the UI on an
	// agent that stopped reading.
	QueueSize int
}

// Run draws the A2UI stream read from in (JSONL, one message or message
// array per line) until the user quits or ctx ends, and writes every event
// to out as JSONL. The stream ending does not quit: the last surface stays
// usable.
func Run(ctx context.Context, in io.Reader, out io.Writer, o RunOptions) error {
	if o.Theme.Renderer() == nil {
		// Color detection must look at the terminal, not at the event pipe.
		o.Theme = theme.New(theme.NewRenderer(screenOrStdout(o.Screen)))
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 256
	}
	events := make(chan Event, o.QueueSize)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		enc := json.NewEncoder(out)
		for e := range events {
			if err := enc.Encode(e); err != nil && o.Logf != nil {
				o.Logf("surface: write event: %v", err)
			}
		}
	}()
	sink := func(e Event) error {
		select {
		case events <- e:
			return nil
		default:
			return errors.New("event queue full: is the agent reading?")
		}
	}

	opts := append([]tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}, mouse.ProgramOptions()...)
	if o.Keyboard != nil {
		opts = append(opts, tea.WithInput(o.Keyboard))
	}
	if o.Screen != nil {
		opts = append(opts, tea.WithOutput(o.Screen))
	}
	p := tea.NewProgram(New(sink, o.Options), opts...)
	go read(in, p.Send)

	_, err := p.Run()
	close(events)
	wg.Wait()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil // cancelled by the caller: a normal end
	}
	return err
}

// read feeds lines to send until in ends. Lines have no length limit: a
// large updateComponents is one line in JSONL.
func read(in io.Reader, send func(tea.Msg)) {
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			send(LineMsg(line))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			send(EndMsg{Err: err})
			return
		}
	}
}

func screenOrStdout(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return os.Stdout
}
