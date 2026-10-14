package kitty

import (
	"strings"
	"testing"
	"unicode"

	"github.com/mattn/go-runewidth"

	"github.com/keyolk/tuikit/render"
)

func isMark(r rune) bool { return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc) }

// marks returns the combining marks on the first cell of a placeholder line.
func marks(line string) []rune {
	var out []rune
	seen := 0
	for _, r := range line {
		switch {
		case r == PlaceholderRune:
			seen++
			if seen > 1 {
				return out
			}
		case seen == 1 && isMark(r):
			out = append(out, r)
		}
	}
	return out
}

func TestDiacriticTableIsDistinctAndBounded(t *testing.T) {
	if MaxIndex != 297 {
		t.Fatalf("table has %d entries, Kitty publishes 297", MaxIndex)
	}
	seen := map[rune]int{}
	for i := range MaxIndex {
		d, ok := Diacritic(i)
		if !ok || !isMark(d) {
			t.Fatalf("Diacritic(%d) = %U, %v", i, d, ok)
		}
		if j, dup := seen[d]; dup {
			t.Fatalf("Diacritic(%d) == Diacritic(%d): two rows would share a position", i, j)
		}
		seen[d] = i
	}
	// Anchors from Kitty's rowcolumn-diacritics.txt, around where ccproxy's
	// copy went wrong (ten extra marks after U+A66F).
	for i, want := range map[int]rune{0: 0x0305, 245: 0xA66F, 246: 0xA67C, 247: 0xA67D, 255: 0xA8E5, 296: 0x1D244} {
		if got, _ := Diacritic(i); got != want {
			t.Errorf("Diacritic(%d) = %U, Kitty's table says %U", i, got, want)
		}
	}
	for _, i := range []int{-1, MaxIndex, 1000} {
		if _, ok := Diacritic(i); ok {
			t.Fatalf("Diacritic(%d) must not clamp", i)
		}
	}
}

func TestPlaceholderShape(t *testing.T) {
	const cols, rows = 6, 4
	block := Placeholder(3, cols, rows)
	if len(block) != rows {
		t.Fatalf("%d lines, want %d", len(block), rows)
	}
	for i, ln := range block {
		if !strings.HasPrefix(ln, "\x1b[38;5;3m") || !strings.HasSuffix(ln, "\x1b[39m") {
			t.Fatalf("line %d lacks the id color frame: %q", i, ln)
		}
		if n := strings.Count(ln, string(PlaceholderRune)); n != cols {
			t.Fatalf("line %d: %d placeholder cells, want %d", i, n, cols)
		}
		want, _ := Diacritic(i)
		if m := marks(ln); len(m) != 1 || m[0] != want {
			t.Fatalf("line %d marks %U, want only the row mark %U", i, m, want)
		}
	}
}

// Regression guard from ccproxy: no column mark per cell, so a wide row
// carries exactly one combining mark.
func TestPlaceholderWideRowHasOneMark(t *testing.T) {
	for _, ln := range Placeholder(5, 130, 3) {
		n := 0
		for _, r := range ln {
			if isMark(r) {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%d marks in a 130-cell row, want 1", n)
		}
	}
}

func TestPlaceholderRowsAndColumnsPast255(t *testing.T) {
	for _, row := range []int{0, 1, 34, 35, 255, 256, 296} {
		block := PlaceholderAt(9, row, 0, 2, 1)
		want, _ := Diacritic(row)
		if len(block) != 1 || marks(block[0])[0] != want {
			t.Fatalf("row %d: block %q, want row mark %U", row, block, want)
		}
	}
	if PlaceholderAt(9, 296, 0, 1, 2) != nil || PlaceholderAt(9, 297, 0, 1, 1) != nil {
		t.Fatal("rows past the table must give nil, not a clamped position")
	}
	for _, col := range []int{1, 255, 256, 296} {
		block := PlaceholderAt(9, 2, col, 3, 1)
		r, _ := Diacritic(2)
		c, _ := Diacritic(col)
		if m := marks(block[0]); len(m) != 2 || m[0] != r || m[1] != c {
			t.Fatalf("col %d: marks %U, want [%U %U]", col, m, r, c)
		}
	}
	if PlaceholderAt(9, 0, 297, 1, 1) != nil {
		t.Fatal("a first column past the table must give nil")
	}
}

func TestPlaceholderIDColors(t *testing.T) {
	if ln := Placeholder(300, 1, 1)[0]; !strings.HasPrefix(ln, "\x1b[38;2;0;1;44m") {
		t.Fatalf("id 300 must be 24-bit fg, got %q", ln)
	}
	ln := Placeholder(0x01000005, 1, 1)[0]
	if !strings.HasPrefix(ln, "\x1b[38;2;0;0;5m") {
		t.Fatalf("low 24 bits as fg, got %q", ln)
	}
	r0, _ := Diacritic(0)
	hi, _ := Diacritic(1)
	if m := marks(ln); len(m) != 3 || m[0] != r0 || m[1] != r0 || m[2] != hi {
		t.Fatalf("id >= 2^24 needs row, column and top-byte marks, got %U", m)
	}
	for _, args := range [][3]int{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}} {
		if Placeholder(uint32(args[0]), args[1], args[2]) != nil {
			t.Fatalf("Placeholder%v must be nil", args)
		}
	}
}

// Every line measures exactly cols cells by the kit's one rule (lipgloss).
func TestPlaceholderMeasuresCols(t *testing.T) {
	for _, cols := range []int{1, 2, 40, 160} {
		for i, ln := range Placeholder(200, cols, SafeRows) {
			if w := render.Width(ln); w != cols {
				t.Fatalf("cols %d row %d: width %d", cols, i, w)
			}
		}
	}
}

// SafeRows follows the table: wcwidth-style tables (go-runewidth here,
// standing in for the C library tmux measures with — not for layout, which
// is render.Width) give marks 0..34 zero width and mark 35 one cell.
func TestSafeRowsMatchesTable(t *testing.T) {
	cond := runewidth.Condition{EastAsianWidth: false}
	for i := range SafeRows {
		if w := cond.RuneWidth(diacritics[i]); w != 0 {
			t.Fatalf("Diacritic(%d) %U is %d wide, inside SafeRows", i, diacritics[i], w)
		}
	}
	if w := cond.RuneWidth(diacritics[SafeRows]); w == 0 {
		t.Fatalf("Diacritic(%d) is zero-width too: SafeRows is too low", SafeRows)
	}
}
