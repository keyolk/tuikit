package kitty

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

var (
	on   = Caps{Supported: true}
	tmux = Caps{Supported: true, InTmux: true, NeedsPassthrough: true, PassthroughMode: "all"}
)

func TestTransmitGolden(t *testing.T) {
	got := on.Transmit(7, []byte("abc"), TransmitOpts{})
	want := "\x1b_Ga=t,t=d,f=100,i=7,q=2,m=0;YWJj\x1b\\"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	got = on.Transmit(2, []byte{1, 2, 3, 4, 5, 6}, TransmitOpts{Format: RGB, Width: 2, Height: 1})
	want = "\x1b_Ga=t,t=d,f=24,i=2,q=2,s=2,v=1,m=0;AQIDBAUG\x1b\\"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// chunks splits a Transmit result into its escapes' "keys;payload" bodies.
func chunks(t *testing.T, seq string) []string {
	t.Helper()
	var out []string
	for _, part := range strings.Split(seq, "\x1b\\") {
		if part == "" {
			continue
		}
		body, ok := strings.CutPrefix(part, "\x1b_G")
		if !ok {
			t.Fatalf("not a graphics escape: %q", part[:min(len(part), 20)])
		}
		out = append(out, body)
	}
	return out
}

func TestTransmitChunkBoundary(t *testing.T) {
	cases := []struct {
		n         int // raw bytes; base64 is 4n/3
		wantSizes []int
	}{
		{3071, []int{4096}},       // 4096 chars after padding
		{3072, []int{4096}},       // exactly one full chunk
		{3073, []int{4096, 4}},    // one byte over
		{6144, []int{4096, 4096}}, // exactly two
	}
	for _, tc := range cases {
		data := bytes.Repeat([]byte{0xAB}, tc.n)
		cs := chunks(t, on.Transmit(9, data, TransmitOpts{}))
		if len(cs) != len(tc.wantSizes) {
			t.Fatalf("%d bytes: %d chunks, want %d", tc.n, len(cs), len(tc.wantSizes))
		}
		var joined strings.Builder
		for i, c := range cs {
			keys, payload, _ := strings.Cut(c, ";")
			last := i == len(cs)-1
			wantKeys := "m=1,q=2"
			switch {
			case i == 0 && last:
				wantKeys = "a=t,t=d,f=100,i=9,q=2,m=0"
			case i == 0:
				wantKeys = "a=t,t=d,f=100,i=9,q=2,m=1"
			case last:
				wantKeys = "m=0,q=2"
			}
			if keys != wantKeys {
				t.Fatalf("%d bytes chunk %d keys %q, want %q", tc.n, i, keys, wantKeys)
			}
			if len(payload) != tc.wantSizes[i] {
				t.Fatalf("%d bytes chunk %d is %d chars, want %d", tc.n, i, len(payload), tc.wantSizes[i])
			}
			joined.WriteString(payload)
		}
		dec, err := base64.StdEncoding.DecodeString(joined.String())
		if err != nil || !bytes.Equal(dec, data) {
			t.Fatalf("%d bytes: chunks do not reassemble (%v)", tc.n, err)
		}
	}
}

// unwrap undoes one tmux passthrough frame and fails on a lone ESC inside.
func unwrap(t *testing.T, s string) string {
	t.Helper()
	inner, ok := strings.CutPrefix(s, "\x1bPtmux;")
	if !ok {
		t.Fatalf("not wrapped: %q", s)
	}
	inner, ok = strings.CutSuffix(inner, "\x1b\\")
	if !ok {
		t.Fatalf("unterminated: %q", s)
	}
	for i := 0; i < len(inner); i++ {
		if inner[i] == 0x1b {
			if i+1 >= len(inner) || inner[i+1] != 0x1b {
				t.Fatalf("undoubled ESC at %d in %q", i, inner)
			}
			i++
		}
	}
	return strings.ReplaceAll(inner, "\x1b\x1b", "\x1b")
}

func TestTmuxWrapDoublesEscape(t *testing.T) {
	got := tmux.Transmit(7, []byte("abc"), TransmitOpts{})
	want := "\x1bPtmux;\x1b\x1b_Ga=t,t=d,f=100,i=7,q=2,m=0;YWJj\x1b\x1b\\\x1b\\"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if unwrap(t, got) != on.Transmit(7, []byte("abc"), TransmitOpts{}) {
		t.Fatal("unwrapped sequence differs from the bare one")
	}
	if s := on.Wrap("\x1b_Ga=d\x1b\\"); s != "\x1b_Ga=d\x1b\\" {
		t.Fatalf("no passthrough needed, but wrapped: %q", s)
	}
	if tmux.Wrap("") != "" {
		t.Fatal("empty input must stay empty")
	}
}

func TestTmuxWrapsEachChunk(t *testing.T) {
	seq := tmux.Transmit(1, bytes.Repeat([]byte{1}, 3*3072), TransmitOpts{})
	if n := strings.Count(seq, "\x1bPtmux;"); n != 3 {
		t.Fatalf("%d passthrough frames, want one per chunk (3)", n)
	}
	for _, frame := range strings.SplitAfter(seq, "\x1b\x1b\\\x1b\\") {
		if frame != "" {
			unwrap(t, frame)
		}
	}
}

func TestTransmitImageEncodesPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	seq, err := on.TransmitImage(3, img)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b_G"), "\x1b\\"), ";")
	data, _ := base64.StdEncoding.DecodeString(payload)
	got, err := png.Decode(bytes.NewReader(data))
	if err != nil || got.Bounds().Dx() != 4 {
		t.Fatalf("payload is not the PNG: %v", err)
	}
}

func TestTransmitFileAndShared(t *testing.T) {
	if got, want := on.TransmitFile(4, "/tmp/a.png", TransmitOpts{}),
		"\x1b_Ga=t,t=f,f=100,i=4,q=2;"+base64.StdEncoding.EncodeToString([]byte("/tmp/a.png"))+"\x1b\\"; got != want {
		t.Fatalf("file: got %q want %q", got, want)
	}
	if got, want := on.TransmitShared(5, "/tuikit-1", 32, TransmitOpts{Format: RGBA, Width: 2, Height: 4}),
		"\x1b_Ga=t,t=s,f=32,i=5,q=2,s=2,v=4,S=32;"+base64.StdEncoding.EncodeToString([]byte("/tuikit-1"))+"\x1b\\"; got != want {
		t.Fatalf("shared: got %q want %q", got, want)
	}
}

func TestPlaceGolden(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"virtual", on.Place(5, 1, 40, 16, PlaceOpts{Virtual: true}), "\x1b_Ga=p,i=5,p=1,c=40,r=16,U=1,C=1,q=2\x1b\\"},
		{"under text", on.Place(5, 0, 3, 2, PlaceOpts{Z: -1}), "\x1b_Ga=p,i=5,c=3,r=2,z=-1,C=1,q=2\x1b\\"},
		{"at a cell", on.Place(5, 2, 3, 2, PlaceOpts{MoveTo: true, Row: 2, Col: 2}),
			"\x1b7\x1b[3;3H\x1b_Ga=p,i=5,p=2,c=3,r=2,C=1,q=2\x1b\\\x1b8"},
		{"at a cell in a tmux pane", tmux.Place(5, 2, 3, 2, PlaceOpts{MoveTo: true, Row: 2, Col: 2, PaneTop: 1, PaneLeft: 10}),
			"\x1bPtmux;\x1b\x1b7\x1b\x1b[4;13H\x1b\x1b_Ga=p,i=5,p=2,c=3,r=2,C=1,q=2\x1b\x1b\\\x1b\x1b8\x1b\\"},
		{"delete", on.Delete(5), "\x1b_Ga=d,d=I,i=5,q=2\x1b\\"},
		{"delete all", on.DeleteAll(), "\x1b_Ga=d,d=A,q=2\x1b\\"},
		{"delete all in tmux", tmux.DeleteAll(), "\x1bPtmux;\x1b\x1b_Ga=d,d=A,q=2\x1b\x1b\\\x1b\\"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestUnsupportedEmitsNothing(t *testing.T) {
	off := Caps{}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	s, err := off.TransmitImage(1, img)
	for name, got := range map[string]string{
		"transmit":  off.Transmit(1, []byte("x"), TransmitOpts{}),
		"image":     s,
		"file":      off.TransmitFile(1, "/a", TransmitOpts{}),
		"shared":    off.TransmitShared(1, "/a", 1, TransmitOpts{}),
		"place":     off.Place(1, 0, 1, 1, PlaceOpts{}),
		"delete":    off.Delete(1),
		"deleteAll": off.DeleteAll(),
		"id 0":      on.Transmit(0, []byte("x"), TransmitOpts{}),
		"empty":     on.Transmit(1, nil, TransmitOpts{}),
		"no size":   on.Place(1, 0, 0, 1, PlaceOpts{}),
	} {
		if got != "" {
			t.Errorf("%s: %q, want empty", name, got)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}
