// Command tuisurf renders an A2UI v0.9 stream in the terminal.
//
// Two transports:
//
//	agent | tuisurf > events.jsonl      # stdin/stdout
//	tuisurf --listen /tmp/ui.sock       # unix socket, one agent connection
//
// In stdin mode the agent owns stdin and stdout, so the TUI reads keys from
// and draws on /dev/tty. Socket mode leaves the terminal alone and is the
// one to prefer for agents: messages and events share one connection.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/keyolk/tuikit/surface"
)

func main() {
	listen := flag.String("listen", "", "serve one agent on this unix socket instead of stdin/stdout")
	logPath := flag.String("log", "", "log file for rejected messages and pane panics (default: a temp file)")
	catalog := flag.Bool("catalog", false, "print the terminal catalog (JSON Schema) for an agent's prompt and exit")
	flag.Parse()
	if *catalog {
		if _, err := os.Stdout.Write(surface.CatalogJSON); err != nil {
			fmt.Fprintln(os.Stderr, "tuisurf:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(*listen, *logPath); err != nil {
		fmt.Fprintln(os.Stderr, "tuisurf:", err)
		os.Exit(1)
	}
}

func run(listen, logPath string) error {
	var lf *os.File
	var err error
	if logPath == "" {
		lf, err = os.CreateTemp("", "tuisurf-*.log")
	} else {
		lf, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}
	if err != nil {
		return err
	}
	defer lf.Close()
	logger := log.New(lf, "", log.LstdFlags)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	opts := surface.RunOptions{Options: surface.Options{Logf: logger.Printf}}

	var in io.Reader
	var out io.Writer
	if listen != "" {
		conn, err := accept(listen)
		if err != nil {
			return err
		}
		defer conn.Close()
		in, out = conn, conn
	} else {
		// The TUI cannot share the agent's pipes: keys come from and the
		// frame goes to the controlling terminal.
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("stdin mode needs a terminal (/dev/tty): %w; use --listen", err)
		}
		defer tty.Close()
		opts.Keyboard, opts.Screen = tty, tty
		in, out = os.Stdin, os.Stdout
	}
	err = surface.Run(ctx, in, out, opts)
	fmt.Fprintln(os.Stderr, "tuisurf: log at", lf.Name())
	return err
}

// accept waits for one agent on path. A stale socket file left by a crashed
// run is removed; any other file at path is an error, never deleted.
func accept(path string) (net.Conn, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if c, err := net.Dial("unix", path); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s is in use by another renderer", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	defer l.Close() // one agent per renderer; the socket file goes with it
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "tuisurf: waiting for an agent on %s\n", path)
	return l.Accept()
}
