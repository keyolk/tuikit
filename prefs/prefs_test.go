package prefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type ui struct {
	Tab    string            `json:"tab"`
	Split  int               `json:"split"`
	SelID  string            `json:"sel_id,omitempty"`
	Folded map[string]bool   `json:"folded,omitempty"`
	Extra  map[string]string `json:"extra,omitempty"`
}

// isolate points both HOME and XDG_STATE_HOME at fresh temp dirs, so no test
// reads or writes the developer's real state.
func isolate(t *testing.T) (home, state string) {
	t.Helper()
	home, state = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", state)
	return home, state
}

func TestPathHonorsXDGAndFallsBackToHome(t *testing.T) {
	home, state := isolate(t)
	if got, want := Path("demo"), filepath.Join(state, "demo", "ui.json"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	t.Setenv("XDG_STATE_HOME", "")
	if got, want := Path("demo"), filepath.Join(home, ".local", "state", "demo", "ui.json"); got != want {
		t.Fatalf("fallback Path = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	isolate(t)
	def := ui{Tab: "list", Split: 40}
	got, err := Load("demo", def)
	if err != nil || got.Tab != "list" || got.Split != 40 {
		t.Fatalf("Load = %+v, %v", got, err)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	isolate(t)
	want := ui{Tab: "detail", Split: 55, SelID: "배포-1", Folded: map[string]bool{"a": true}}
	msg := SaveCmd("demo", want)()
	saved, ok := msg.(Saved)
	if !ok || saved.Err != nil || saved.App != "demo" {
		t.Fatalf("SaveCmd → %#v", msg)
	}
	got, err := Load("demo", ui{Tab: "list"})
	if err != nil || got.Tab != "detail" || got.Split != 55 || got.SelID != "배포-1" || !got.Folded["a"] {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(Path("demo"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v; want 0600", fi.Mode().Perm(), err)
		}
	}
	// No temp files left behind.
	entries, err := os.ReadDir(filepath.Dir(Path("demo")))
	if err != nil || len(entries) != 1 {
		t.Fatalf("state dir holds %d entries, want only ui.json", len(entries))
	}
}

func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(filepath.Dir(Path("demo")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("demo"), []byte(`{"tab":"detail"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load("demo", ui{Tab: "list", Split: 40})
	if err != nil || got.Tab != "detail" || got.Split != 40 {
		t.Fatalf("Load = %+v, %v; want Split default kept", got, err)
	}
}

// Decoding over a shallow copy of def would write into def's map.
func TestLoadDoesNotMutateDefault(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(filepath.Dir(Path("demo")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("demo"), []byte(`{"extra":{"k":"from-file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	def := ui{Extra: map[string]string{"k": "default"}}
	got, err := Load("demo", def)
	if err != nil || got.Extra["k"] != "from-file" {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if def.Extra["k"] != "default" {
		t.Fatalf("caller's default map was overwritten: %v", def.Extra)
	}
}

func TestLoadCorruptFileReturnsDefaultAndError(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(filepath.Dir(Path("demo")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("demo"), []byte(`{"tab":`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load("demo", ui{Tab: "list"})
	if err == nil || got.Tab != "list" {
		t.Fatalf("Load = %+v, %v; want default and an error", got, err)
	}
}

// The value is captured when SaveCmd is called, not when the command runs.
func TestSaveCmdSnapshotsValueAtCallTime(t *testing.T) {
	isolate(t)
	v := ui{Folded: map[string]bool{"before": true}}
	cmd := SaveCmd("demo", v)
	v.Folded["after"] = true
	if msg := cmd().(Saved); msg.Err != nil {
		t.Fatal(msg.Err)
	}
	got, err := Load("demo", ui{})
	if err != nil || got.Folded["after"] {
		t.Fatal("SaveCmd encoded the value when it ran, not when it was created")
	}
}

func TestSaveCmdReportsErrors(t *testing.T) {
	_, state := isolate(t)
	// A file where the app directory should be makes MkdirAll fail.
	if err := os.WriteFile(filepath.Join(state, "demo"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := SaveCmd("demo", ui{})().(Saved); msg.Err == nil {
		t.Fatal("expected an error when the state dir cannot be created")
	}
	if msg := SaveCmd("demo", func() {})().(Saved); msg.Err == nil {
		t.Fatal("expected an encode error for an unencodable value")
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	isolate(t)
	// A directory where the file should be: ReadFile fails with something
	// other than not-exist.
	if err := os.MkdirAll(Path("demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("demo", ui{}); err == nil {
		t.Fatal("expected an error for an unreadable state file")
	}
}
