// Package theme names colors by role, not by hue (engine principle E10), and
// degrades under NO_COLOR and non-UTF-8 locales.
//
// Render code asks for Warn or Subtle, never "#F59E0B" or styCyan, so the
// palette, light/dark adaptation and NO_COLOR are decided here once. Ported
// from dayx and okx internal/tui/styles.go, which the fleet audit named the
// reference copies.
package theme

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/toast"
	// termenv is lipgloss's own dependency; it is imported only for the
	// profile constant SetColorProfile takes, so it adds no module.
	"github.com/muesli/termenv"
)

// Roles is the palette. Each role adapts to the terminal background.
type Roles struct {
	Text        lipgloss.AdaptiveColor // body text
	Muted       lipgloss.AdaptiveColor // secondary metadata
	Subtle      lipgloss.AdaptiveColor // rules, hints, dimmed frame
	Accent      lipgloss.AdaptiveColor // titles, the focused thing
	Info        lipgloss.AdaptiveColor
	Success     lipgloss.AdaptiveColor
	Warn        lipgloss.AdaptiveColor
	Error       lipgloss.AdaptiveColor
	Selected    lipgloss.AdaptiveColor // selected-row background
	Border      lipgloss.AdaptiveColor
	BorderFocus lipgloss.AdaptiveColor
	DiffAdded   lipgloss.AdaptiveColor
	DiffRemoved lipgloss.AdaptiveColor
}

// DefaultRoles is the dayx/okx palette (dark values) with light-background
// counterparts dark enough to read on white.
func DefaultRoles() Roles {
	c := func(light, dark string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Light: light, Dark: dark}
	}
	return Roles{
		Text:        c("#1F2937", "#E5E7EB"),
		Muted:       c("#6B7280", "#9CA3AF"),
		Subtle:      c("#9CA3AF", "#4B5563"),
		Accent:      c("#7C3AED", "#A78BFA"),
		Info:        c("#0369A1", "#38BDF8"),
		Success:     c("#15803D", "#22C55E"),
		Warn:        c("#B45309", "#F59E0B"),
		Error:       c("#B91C1C", "#EF4444"),
		Selected:    c("#E5E7EB", "#374151"),
		Border:      c("#D1D5DB", "#4B5563"),
		BorderFocus: c("#0369A1", "#38BDF8"),
		DiffAdded:   c("#15803D", "#4ADE80"),
		DiffRemoved: c("#B91C1C", "#F87171"),
	}
}

// Theme is the palette plus ready-made styles, all bound to one renderer.
type Theme struct {
	Roles  Roles
	Glyphs Glyphs

	Text, Muted, Subtle, Accent lipgloss.Style
	Info, Success, Warn, Error  lipgloss.Style
	DiffAdded, DiffRemoved      lipgloss.Style
	Title, Key, Bold            lipgloss.Style
	Selected                    lipgloss.Style // the cursor row
	Border, BorderFocus         lipgloss.Style // border color only
	Box, BoxFocus               lipgloss.Style // bordered box for panes/modals

	renderer *lipgloss.Renderer
	noColor  bool
}

// NewRenderer returns a renderer for w that honours NO_COLOR the way New
// does. Bubble Tea writes to stdout by default; pass os.Stderr if the program
// does.
func NewRenderer(w io.Writer) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)
	applyNoColor(r)
	return r
}

// applyNoColor reports whether NO_COLOR is set (any value, per no-color.org)
// and, if so, picks a profile that still draws attributes.
//
// termenv maps NO_COLOR to its Ascii profile, and Ascii drops every SGR
// sequence — bold and reverse included — so a selection drawn with reverse
// video silently disappears. dayx forced Ascii under NO_COLOR believing bold
// and reverse survive; they do not. Instead, when the output is a terminal,
// the profile is set to plain ANSI and the theme builds its styles without
// any color, which is what NO_COLOR asks for: no color, attributes allowed.
func applyNoColor(r *lipgloss.Renderer) bool {
	if os.Getenv("NO_COLOR") == "" {
		return false
	}
	// Output().ColorProfile() is the TTY-based detection that ignores
	// NO_COLOR; Ascii there means "not a terminal", which stays plain.
	if r.Output().ColorProfile() != termenv.Ascii {
		r.SetColorProfile(termenv.ANSI)
	}
	return true
}

// New builds the theme against r (lipgloss.DefaultRenderer() when nil) with
// DefaultRoles and glyphs detected for no particular app. Under NO_COLOR the
// styles carry no color at all but keep bold and reverse: they carry the
// selection and titles, and a cursor row that relied on a background color
// alone would vanish (see applyNoColor for why the profile is not Ascii).
func New(r *lipgloss.Renderer) Theme {
	return NewWith(r, DefaultRoles(), DetectGlyphs(""))
}

// NewWith is New with an explicit palette and glyph set.
func NewWith(r *lipgloss.Renderer, roles Roles, g Glyphs) Theme {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	noColor := applyNoColor(r)
	fg := func(c lipgloss.AdaptiveColor) lipgloss.Style {
		if noColor {
			return r.NewStyle()
		}
		return r.NewStyle().Foreground(c)
	}
	box := func(c lipgloss.AdaptiveColor) lipgloss.Style {
		s := r.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
		if noColor {
			return s
		}
		return s.BorderForeground(c)
	}
	t := Theme{
		Roles: roles, Glyphs: g, renderer: r, noColor: noColor,
		Text: fg(roles.Text), Muted: fg(roles.Muted), Subtle: fg(roles.Subtle), Accent: fg(roles.Accent),
		Info: fg(roles.Info), Success: fg(roles.Success), Warn: fg(roles.Warn), Error: fg(roles.Error).Bold(true),
		DiffAdded: fg(roles.DiffAdded), DiffRemoved: fg(roles.DiffRemoved),
		Title:       fg(roles.Accent).Bold(true),
		Key:         fg(roles.Info).Bold(true),
		Bold:        r.NewStyle().Bold(true),
		Border:      fg(roles.Border),
		BorderFocus: fg(roles.BorderFocus),
		Box:         box(roles.Border),
		BoxFocus:    box(roles.BorderFocus),
	}
	if noColor || r.ColorProfile() == termenv.Ascii {
		t.Selected = r.NewStyle().Bold(true).Reverse(true)
	} else {
		t.Selected = r.NewStyle().Bold(true).Background(roles.Selected)
	}
	if g.ASCII {
		// Rounded corners are box drawing; an ASCII terminal gets +-| instead.
		t.Box = t.Box.Border(asciiBorder)
		t.BoxFocus = t.BoxFocus.Border(asciiBorder)
	}
	return t
}

var asciiBorder = lipgloss.Border{
	Top: "-", Bottom: "-", Left: "|", Right: "|",
	TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
}

// Renderer is the renderer the styles are bound to.
func (t Theme) Renderer() *lipgloss.Renderer { return t.renderer }

// NoColor reports whether NO_COLOR stripped the theme's colors.
func (t Theme) NoColor() bool { return t.noColor }

// plain reports whether surfaces should draw words, not glyphs: the ASCII
// set, or the zero Theme (not built by New), whose empty Glyphs is no set
// at all — a model literal in a test should not get ↵ and ✓.
func (t Theme) plain() bool { return t.Glyphs.ASCII || t.renderer == nil }

// ToastStyles adapts the theme for toast.View.
func (t Theme) ToastStyles() toast.Styles {
	return toast.Styles{
		Info: t.Info, Success: t.Success, Warn: t.Warn, Error: t.Error, Progress: t.Accent,
		ASCII: t.plain(), Ellipsis: t.Glyphs.Ellipsis,
	}
}

// KeyNames is how keys are displayed: glyphs (enter → ↵, up → ↑) with the
// Unicode glyph set, words with the ASCII one and with the zero Theme.
func (t Theme) KeyNames() keys.Names {
	return keys.Names{Glyphs: !t.plain()}
}

// FooterStyle adapts the theme for keys.Keymap.Footer.
func (t Theme) FooterStyle() keys.FooterStyle {
	return keys.FooterStyle{Key: t.Key, Desc: t.Muted, More: t.Subtle, Names: t.KeyNames(), Ellipsis: t.Glyphs.Ellipsis}
}

// HelpStyle adapts the theme for keys.RenderHelp.
func (t Theme) HelpStyle() keys.HelpStyle {
	return keys.HelpStyle{Title: t.Title, Key: t.Key, Desc: t.Text, Muted: t.Muted, Names: t.KeyNames(), Ellipsis: t.Glyphs.Ellipsis}
}
