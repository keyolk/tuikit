// Package prefs persists UI state worth keeping across restarts — tab,
// filter, layout mode, split ratio, last selection ID — to
// $XDG_STATE_HOME/<app>/ui.json (engine principle E12).
//
// Writes are atomic (temp file in the same directory, then rename) and run in
// a tea.Cmd, off the UI loop. ghx wrote its repo store non-atomically inside
// Update and dropped the error; a crash mid-write then left a truncated file
// that failed to parse on the next start.
//
// Writes are ordered: an older write never lands over a newer one, however
// the command goroutines are scheduled. Saver adds a debounce and a Flush for
// quit on top; SaveCmd is the one-shot form.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// Path is the state file for app: $XDG_STATE_HOME/<app>/ui.json, falling back
// to ~/.local/state/<app>/ui.json. State, not config: it is written by the
// program, not edited by the user.
func Path(app string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, app, "ui.json")
}

// Load reads app's state over a copy of def, so fields missing from the file
// (added in a newer version) keep their defaults. A missing file returns def
// and a nil error. A corrupt file returns def and the error; the caller
// decides whether to tell the user — losing UI state should never stop the
// app from starting. app "" means persistence is off: def, nil.
//
// def is deep-copied (through JSON) before the file is decoded over it:
// decoding straight into a copy of def would write into the very maps and
// slices def points at, changing the caller's defaults.
func Load[T any](app string, def T) (T, error) {
	if app == "" {
		return def, nil
	}
	data, err := os.ReadFile(Path(app))
	if errors.Is(err, os.ErrNotExist) {
		return def, nil
	}
	if err != nil {
		return def, fmt.Errorf("prefs: %w", err)
	}
	v, err := clone(def)
	if err != nil {
		return def, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return def, fmt.Errorf("prefs %s: %w", Path(app), err)
	}
	return v, nil
}

func clone[T any](v T) (T, error) {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return v, fmt.Errorf("prefs: encode defaults: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return v, fmt.Errorf("prefs: copy defaults: %w", err)
	}
	return out, nil
}

// Saved reports the outcome of a write. Surface Err (a toast); do not drop
// it. Final marks the write a Saver.Flush asked for: quit on it.
type Saved struct {
	App   string
	Err   error
	Final bool
}

// SaveCmd returns a command that writes v for app atomically with mode 0600.
// app "" writes nothing and returns nil.
//
// v is encoded now, when SaveCmd is called from Update, not later when the
// command runs: the command runs on another goroutine, and encoding there
// would read maps the next Update may already be writing.
func SaveCmd[T any](app string, v T) tea.Cmd {
	if app == "" {
		return nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		err = fmt.Errorf("prefs: encode: %w", err)
		return func() tea.Msg { return Saved{App: app, Err: err} }
	}
	return writeCmd(app, append(data, '\n'), false)
}

// Writes are ordered by a sequence number taken when the command is built,
// in Update, which Bubble Tea runs on one goroutine: a later Update always
// gets a larger number. Commands run on their own goroutines in any order,
// so each write takes writeMu and skips itself when a newer write for the
// same path already landed — an older write never overwrites a newer one.
// This state is process-wide because the files are.
var (
	seq     atomic.Uint64
	writeMu sync.Mutex
	landed  = map[string]uint64{} // path → seq of the newest write on disk
)

func writeCmd(app string, data []byte, final bool) tea.Cmd {
	path, n := Path(app), seq.Add(1)
	return func() tea.Msg {
		writeMu.Lock()
		defer writeMu.Unlock()
		if landed[path] > n {
			return Saved{App: app, Final: final} // superseded; the newer data is on disk
		}
		err := writeAtomic(path, data)
		if err == nil {
			landed[path] = n
		}
		return Saved{App: app, Err: err, Final: final}
	}
}

// writeAtomic writes data to a temp file beside path and renames it over
// path, so a reader (or a crash) sees either the old file or the new one,
// never half of one. The temp file must be in the same directory: rename is
// only atomic within one filesystem.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prefs: %w", err)
	}
	f, err := os.CreateTemp(dir, ".ui-*.json")
	if err != nil {
		return fmt.Errorf("prefs: %w", err)
	}
	tmp := f.Name()
	write := func() error {
		// CreateTemp already uses 0600; Chmod states it rather than relying
		// on that detail.
		if err := f.Chmod(0o600); err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
		return f.Sync()
	}
	if err := write(); err != nil {
		// Joined so a failed cleanup is reported too, not silently dropped.
		return fmt.Errorf("prefs: %w", errors.Join(err, f.Close(), os.Remove(tmp)))
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("prefs: %w", errors.Join(err, os.Remove(tmp)))
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("prefs: %w", errors.Join(err, os.Remove(tmp)))
	}
	return nil
}
