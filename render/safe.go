package render

import (
	"log"
	"runtime/debug"
)

// Safe runs f and returns its output; if f panics, the panic and its stack go
// to logf and a one-line fallback fitted to w×h is returned instead (E4).
//
// One pane's bad data then costs that pane, not the app: Bubble Tea restores
// the terminal on a panic in View, but the whole session is gone and the user
// is left with a stack trace instead of the other panes. logf defaults to
// log.Printf; point it at a file, since stderr is the alt screen.
func Safe(name string, w, h int, logf func(string, ...any), f func() string) (out string) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if logf == nil {
			logf = log.Printf
		}
		logf("render %s panicked: %v\n%s", name, r, debug.Stack())
		out = FitLines("⚠ "+name+" failed to draw — see log", w, max(h, 1))
	}()
	return f()
}
