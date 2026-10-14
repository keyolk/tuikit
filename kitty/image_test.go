package kitty

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/keyolk/tuikit/render"
)

func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const hangulAlt = "이미지를 표시할 수 없습니다 — 터미널이 그래픽을 지원하지 않아요"

func assertBox(t *testing.T, view string, cols, rows int) []string {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != rows {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), rows, view)
	}
	for i, l := range lines {
		if w := render.Width(l); w != cols {
			t.Fatalf("line %d is %d cells, want %d: %q", i, w, cols, l)
		}
	}
	return lines
}

func TestImageViewIsExactlyTheBox(t *testing.T) {
	src := Source{Key: "g", Img: gradient(8, 8)}
	for _, size := range [][2]int{{1, 1}, {9, 3}, {40, 16}, {7, SafeRows + 5}} {
		im := Image{ID: 1, Source: src, Cols: size[0], Rows: size[1], Alt: hangulAlt}
		lines := assertBox(t, im.View(on), size[0], size[1])
		for i, l := range lines {
			hasCells := strings.ContainsRune(l, PlaceholderRune)
			if hasCells != (i < SafeRows) {
				t.Fatalf("%v row %d: placeholder=%v", size, i, hasCells)
			}
		}
		// The same box, unsupported: the Hangul alt text, cut to the box.
		alt := assertBox(t, im.View(Caps{}), size[0], size[1])
		if strings.ContainsRune(strings.Join(alt, ""), PlaceholderRune) {
			t.Fatalf("%v: unsupported terminal got placeholder cells", size)
		}
	}
}

func TestImageUnsupportedShowsAlt(t *testing.T) {
	im := Image{ID: 1, Source: Source{Data: []byte("x")}, Cols: 9, Rows: 6, Alt: hangulAlt}
	got := strings.Join(assertBox(t, im.View(Caps{}), 9, 6), "\n")
	for _, word := range []string{"이미지를", "표시할", "없습니다"} {
		if !strings.Contains(got, word) {
			t.Fatalf("alt word %q missing:\n%s", word, got)
		}
	}
	// No pixels, or no id, also means alt even where graphics work.
	for _, im := range []Image{{ID: 1, Cols: 9, Rows: 2, Alt: "없음"}, {Source: Source{Data: []byte("x")}, Cols: 9, Rows: 2, Alt: "없음"}} {
		if v := im.View(on); !strings.HasPrefix(v, "없음") || im.Shown(on) {
			t.Fatalf("%+v drew %q", im, v)
		}
	}
	if (Image{ID: 1, Source: Source{Path: "/a"}, Cols: 0, Rows: 3}).View(on) != "" {
		t.Fatal("no columns, no view")
	}
}

func TestImageValueCopyIsIndependent(t *testing.T) {
	data := pngBytes(t, gradient(4, 4))
	orig := append([]byte(nil), data...)
	a := Image{ID: 1, Source: Source{Data: data}, Cols: 10, Rows: 4, Alt: "a"}
	before := a.View(on)

	b := a
	b.Cols, b.Rows, b.ID, b.Alt = 3, 2, 2, "b"
	b.Source.Key = "other"
	if a.Cols != 10 || a.ID != 1 || a.Alt != "a" || a.Source.Key != "" || a.View(on) != before {
		t.Fatalf("changing the copy changed the original: %+v", a)
	}
	var buf bytes.Buffer
	if msg := b.Cmds(a, on, NewOut(&buf), CellSize{})().(Wrote); msg.Err != nil {
		t.Fatal(msg.Err)
	}
	if !bytes.Equal(data, orig) || a.View(on) != before {
		t.Fatal("running the copy's commands changed the shared source or the original")
	}
}

func run(t *testing.T, im, prev Image, c Caps) (string, Wrote, bool) {
	t.Helper()
	var buf bytes.Buffer
	cmd := im.Cmds(prev, c, NewOut(&buf), CellSize{W: 8, H: 16})
	if cmd == nil {
		return "", Wrote{}, false
	}
	return buf.String() + "", cmd().(Wrote), true
}

func TestImageCmds(t *testing.T) {
	data := pngBytes(t, gradient(6, 6))
	base := Image{ID: 4, Source: Source{Data: data}, Cols: 10, Rows: 5}

	// Appearing: transmit, then the virtual placement the cells point at.
	var buf bytes.Buffer
	msg := base.Cmds(Image{}, on, NewOut(&buf), CellSize{})().(Wrote)
	out := buf.String()
	if msg.ID != 4 || msg.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if !strings.HasPrefix(out, "\x1b_Ga=t,t=d,f=100,i=4,q=2,") || !strings.HasSuffix(out, on.Place(4, 1, 10, 5, PlaceOpts{Virtual: true})) {
		t.Fatalf("appear wrote %q", out[:min(len(out), 60)])
	}
	if strings.Contains(out, "a=d") {
		t.Fatal("nothing to delete on first show")
	}

	if _, _, ok := run(t, base, base, on); ok {
		t.Fatal("unchanged image must give no command")
	}
	same := base
	same.Source.Data = append([]byte(nil), data...) // equal bytes, new slice
	same.Alt = "changed alt only"
	if _, _, ok := run(t, same, base, on); ok {
		t.Fatal("equal bytes must not resend")
	}

	resized := base
	resized.Cols = 12
	buf.Reset()
	resized.Cmds(base, on, NewOut(&buf), CellSize{})()
	if out := buf.String(); strings.Contains(out, "a=d") || !strings.Contains(out, "c=12,r=5,U=1") {
		t.Fatalf("resize must resend and re-place without deleting: %q", out)
	}

	moved := base
	moved.ID = 9
	buf.Reset()
	moved.Cmds(base, on, NewOut(&buf), CellSize{})()
	if out := buf.String(); !strings.HasPrefix(out, on.Delete(4)) || !strings.Contains(out, "i=9") {
		t.Fatalf("new id must delete the old: %q", out[:min(len(out), 60)])
	}

	hidden := base
	hidden.Cols = 0
	buf.Reset()
	msg = hidden.Cmds(base, on, NewOut(&buf), CellSize{})().(Wrote)
	if buf.String() != on.Delete(4) || msg.ID != 4 {
		t.Fatalf("hide wrote %q (%+v)", buf.String(), msg)
	}

	if _, _, ok := run(t, base, Image{}, Caps{}); ok {
		t.Fatal("unsupported terminal must give no command")
	}
	if _, _, ok := run(t, hidden, Image{}, on); ok {
		t.Fatal("never shown and still hidden: no command")
	}

	// Inside tmux everything arrives wrapped.
	buf.Reset()
	base.Cmds(Image{}, tmux, NewOut(&buf), CellSize{})()
	if !strings.HasPrefix(buf.String(), "\x1bPtmux;") {
		t.Fatal("tmux caps must wrap")
	}
}

func TestImageCmdsShrinksDecodedSources(t *testing.T) {
	big := gradient(2000, 1000)
	var buf bytes.Buffer
	im := Image{ID: 1, Source: Source{Key: "big", Img: big}, Cols: 20, Rows: 5}
	if msg := im.Cmds(Image{}, on, NewOut(&buf), CellSize{W: 10, H: 20})().(Wrote); msg.Err != nil {
		t.Fatal(msg.Err)
	}
	if n := buf.Len(); n > 20000 {
		t.Fatalf("a 2000x1000 image for a 20x5 box sent %d bytes", n)
	}
	if _, _, ok := run(t, im, im, on); ok {
		t.Fatal("same Key must not resend")
	}
	keyless := Image{ID: 1, Source: Source{Img: big}, Cols: 20, Rows: 5}
	if _, _, ok := run(t, keyless, keyless, on); !ok {
		t.Fatal("an Img without a Key cannot be compared and is resent")
	}
}

func TestImageCmdsReportsErrors(t *testing.T) {
	prev := Image{ID: 2, Source: Source{Key: "old", Data: []byte("\x89PNG\r\n\x1a\nold")}, Cols: 4, Rows: 2}
	bad := Image{ID: 3, Source: Source{Data: []byte("not an image")}, Cols: 4, Rows: 2}
	var buf bytes.Buffer
	msg := bad.Cmds(prev, on, NewOut(&buf), CellSize{})().(Wrote)
	if msg.Err == nil || msg.ID != 3 {
		t.Fatalf("decode failure must be reported: %+v", msg)
	}
	if buf.String() != on.Delete(2) {
		t.Fatalf("the old image must still be deleted, wrote %q", buf.String())
	}

	ok := Image{ID: 1, Source: Source{Path: "/tmp/x.png"}, Cols: 4, Rows: 2}
	if msg := ok.Cmds(Image{}, on, Out{}, CellSize{})().(Wrote); !errors.Is(msg.Err, ErrNoOutput) {
		t.Fatalf("zero Out must report, not drop: %+v", msg)
	}
	if msg := ok.Cmds(Image{}, on, NewOut(failWriter{}), CellSize{})().(Wrote); msg.Err == nil {
		t.Fatal("write error must be reported")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("tty gone") }

func TestImageHideAndRebind(t *testing.T) {
	im := Image{ID: 6, Source: Source{Path: "/a.png"}, Cols: 50, Rows: 40}
	var buf bytes.Buffer
	out := NewOut(&buf)
	im.RebindCmd(on, out)()
	if buf.String() != on.Place(6, 1, 50, SafeRows, PlaceOpts{Virtual: true}) {
		t.Fatalf("rebind wrote %q (rows capped at SafeRows)", buf.String())
	}
	buf.Reset()
	im.HideCmd(on, out)()
	if buf.String() != on.Delete(6) {
		t.Fatalf("hide wrote %q", buf.String())
	}
	if im.HideCmd(Caps{}, out) != nil || im.RebindCmd(Caps{}, out) != nil {
		t.Fatal("unsupported: no commands")
	}
}

func TestOutCopiesShareOneLock(t *testing.T) {
	var buf bytes.Buffer
	out := NewOut(&buf)
	seq := on.Transmit(1, bytes.Repeat([]byte{7}, 5000), TransmitOpts{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func(o Out) { // a copy, as a command closure holds it
			defer wg.Done()
			if err := o.Write(seq); err != nil {
				t.Error(err)
			}
		}(out)
	}
	wg.Wait()
	if buf.String() != strings.Repeat(seq, 8) {
		t.Fatal("writes interleaved")
	}
	if out.WriteCmd("") != nil {
		t.Fatal("empty write: no command")
	}
	if msg := out.WriteCmd("x")().(Wrote); msg.Err != nil {
		t.Fatal(msg.Err)
	}
}

func TestToPNGAndDownscale(t *testing.T) {
	p := pngBytes(t, gradient(3, 3))
	if got, err := ToPNG(p); err != nil || &got[0] != &p[0] {
		t.Fatal("PNG must pass through untouched")
	}
	if _, err := ToPNG([]byte("garbage")); err == nil {
		t.Fatal("undecodable input must be an error, not passed on")
	}
	small := gradient(10, 10)
	if Downscale(small, 20, 20) != image.Image(small) {
		t.Fatal("never upscale")
	}
	d := Downscale(gradient(400, 100), 40, 40)
	if b := d.Bounds(); b.Dx() != 40 || b.Dy() != 10 {
		t.Fatalf("downscaled to %v, want 40x10", b)
	}
}
