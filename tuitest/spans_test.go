package tuitest

import (
	"slices"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestReversedSpansGivesCellPositions(t *testing.T) {
	r := lipgloss.NewRenderer(nil)
	r.SetColorProfile(termenv.ANSI)
	rev := r.NewStyle().Reverse(true)
	frame := "ab" + rev.Render("cd") + "ef\n" +
		"한글" + rev.Render("x") + "\x1b]8;;https://x.io\x1b\\y\x1b]8;;\x1b\\" + rev.Render("zz") + "\n" +
		"\x1b[7mrow2\nrow3\x1b[0m tail"
	got := ReversedSpans(frame)
	want := []Span{
		{Row: 0, X0: 2, X1: 4, Text: "cd"},
		{Row: 1, X0: 4, X1: 5, Text: "x"},  // Hangul is two cells each
		{Row: 1, X0: 6, X1: 8, Text: "zz"}, // the link's escape takes no cell
		{Row: 2, X0: 0, X1: 4, Text: "row2"},
		{Row: 3, X0: 0, X1: 4, Text: "row3"}, // reverse carried over the newline
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ReversedSpans =\n%+v\nwant\n%+v", got, want)
	}
	var texts []string
	for _, s := range got {
		texts = append(texts, s.Text)
	}
	if rv := Reversed(frame); !slices.Equal(rv, texts) {
		t.Fatalf("Reversed %q disagrees with the spans %q", rv, texts)
	}
}
