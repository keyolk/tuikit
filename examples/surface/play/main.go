// Command play streams an A2UI JSONL file to tuisurf one line at a time, the
// way an agent would: the first lines build the surface, the later ones are
// incremental updates that arrive while the user looks at it.
//
//	go run ./examples/surface/play | go run ./cmd/tuisurf
//	go run ./cmd/tuisurf --listen /tmp/surf.sock     # in one pane
//	go run ./examples/surface/play --socket /tmp/surf.sock   # in another
//
// With --socket, the events the renderer sends back (merge, refresh,
// comment) are printed to stderr as they arrive. Fake data only: nothing
// here touches the network.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

func main() {
	file := flag.String("file", "examples/surface/pr-dashboard.jsonl", "JSONL to stream")
	socket := flag.String("socket", "", "connect to tuisurf --listen at this path instead of writing stdout")
	delay := flag.Duration("delay", 1500*time.Millisecond, "pause before each update line")
	initial := flag.Int("initial", 3, "lines sent at once before the paced updates")
	flag.Parse()
	if err := play(*file, *socket, *delay, *initial); err != nil {
		fmt.Fprintln(os.Stderr, "play:", err)
		os.Exit(1)
	}
}

func play(file, socket string, delay time.Duration, initial int) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	var out io.Writer = os.Stdout
	done := make(chan struct{})
	if socket != "" {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return err
		}
		defer conn.Close()
		out = conn
		go func() {
			defer close(done)
			sc := bufio.NewScanner(conn)
			for sc.Scan() {
				fmt.Fprintln(os.Stderr, "event:", sc.Text())
			}
		}()
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 16<<20)
	for n := 0; sc.Scan(); n++ {
		if n >= initial {
			time.Sleep(delay)
		}
		if _, err := fmt.Fprintln(out, sc.Text()); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if socket == "" {
		// Closing stdin is fine: the renderer toasts "stream ended" and keeps
		// the last surface usable. Waiting instead would hang the pipeline
		// after the user quits, since nothing tells a writer its reader left
		// until it writes.
		return nil
	}
	<-done // until the renderer quits and closes the connection
	return nil
}
