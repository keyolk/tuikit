package kitty

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFit(t *testing.T) {
	cases := []struct {
		name               string
		imgW, imgH, mc, mr int
		cw, ch             int
		wantCols, wantRows int
	}{
		{"square fills width, half the rows", 100, 100, 40, 40, 10, 20, 40, 20},
		{"limited by rows", 100, 100, 80, 10, 10, 20, 20, 10},
		{"scales up", 10, 20, 40, 40, 10, 20, 40, 40},
		{"default cell when zero", 100, 100, 40, 40, 0, 0, 40, 20},
		{"default cell when negative", 100, 100, 40, 40, -1, 20, 40, 20},
		{"square cells", 100, 50, 40, 40, 8, 8, 40, 20},
		{"exact fit does not floor short", 300, 600, 3, 3, 100, 200, 3, 3},
		{"very wide keeps one row", 1000, 1, 40, 40, 10, 20, 40, 1},
		{"very tall keeps one column", 1, 1000, 40, 40, 10, 20, 1, 40},
		{"one cell box", 500, 300, 1, 1, 10, 20, 1, 1},
		{"unknown size gets the box", 0, 100, 30, 12, 10, 20, 30, 12},
		{"negative size gets the box", -5, -5, 30, 12, 10, 20, 30, 12},
		{"no room", 100, 100, 0, 10, 10, 20, 0, 10},
		{"negative room", 100, 100, -3, -3, 10, 20, 0, 0},
	}
	for _, tc := range cases {
		c, r := Fit(tc.imgW, tc.imgH, tc.mc, tc.mr, tc.cw, tc.ch)
		if c != tc.wantCols || r != tc.wantRows {
			t.Errorf("%s: Fit = %dx%d, want %dx%d", tc.name, c, r, tc.wantCols, tc.wantRows)
		}
	}
}

func TestFitStaysInBoxAndKeepsAspect(t *testing.T) {
	for w := 1; w <= 2000; w += 137 {
		for h := 1; h <= 2000; h += 191 {
			c, r := Fit(w, h, 50, 20, 9, 18)
			if c < 1 || r < 1 || c > 50 || r > 20 {
				t.Fatalf("%dx%d px: %dx%d outside 50x20", w, h, c, r)
			}
			if c < 50 && r < 20 && c > 1 && r > 1 {
				t.Fatalf("%dx%d px: %dx%d touches neither side", w, h, c, r)
			}
		}
	}
}

func TestCellSizeCmdReportsErrorWithDefault(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	msg := CellSizeCmd(w.Fd())().(CellSizeMsg)
	if msg.Err == nil || msg.Size != DefaultCellSize() {
		t.Fatalf("a pipe has no window size: %+v", msg)
	}
}

func TestParseCellSize(t *testing.T) {
	if c, ok := ParseCellSize([]byte("\x1b[6;20;10t")); !ok || c != (CellSize{W: 10, H: 20}) {
		t.Fatalf("got %+v %v", c, ok)
	}
	for _, bad := range []string{"\x1b[4;600;800t", "\x1b[6;0;10t", "\x1b[6;20t", "\x1b[6;a;10t", "\x1b[6;20;10u", "6;20;10t", ""} {
		if _, ok := ParseCellSize([]byte(bad)); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestParseKeyboardFlags(t *testing.T) {
	if f, ok := ParseKeyboardFlags([]byte("\x1b[?31u")); !ok || f != 31 {
		t.Fatalf("got %d %v", f, ok)
	}
	if f, ok := ParseKeyboardFlags([]byte("\x1b[?0u")); !ok || f != 0 {
		t.Fatalf("flags 0 still means supported: %d %v", f, ok)
	}
	for _, bad := range []string{"\x1b[?u", "\x1b[31u", "\x1b[?1;2u", "\x1b[?1t"} {
		if _, ok := ParseKeyboardFlags([]byte(bad)); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

// unknownCSISequenceMsg mirrors Bubble Tea v1's unexported type, which is
// how a reply reaches Update.
type unknownCSISequenceMsg []byte

func TestCSIReply(t *testing.T) {
	b, ok := CSIReply(unknownCSISequenceMsg("\x1b[6;18;9t"))
	if !ok {
		t.Fatal("reply not recognised")
	}
	if c, ok := ParseCellSize(b); !ok || c != (CellSize{W: 9, H: 18}) {
		t.Fatalf("got %+v", c)
	}
	for _, msg := range []tea.Msg{nil, tea.KeyMsg{}, []byte("\x1b[6;1;1t"), "\x1b[6;1;1t", unknownCSISequenceMsg("x")} {
		if _, ok := CSIReply(msg); ok {
			t.Errorf("%T %v taken for a reply", msg, msg)
		}
	}
}
