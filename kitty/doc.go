// Package kitty speaks the Kitty graphics protocol from a Bubble Tea app:
// detection (Kitty, Ghostty, WezTerm, also behind tmux), escape encoding,
// Unicode placeholders, size math, and an Image component that is a value.
//
// # No package state
//
// Nothing here is cached behind a sync.Once. The app calls Detect once, in
// main, with the environment and a tmux querier, and keeps the Caps value in
// its model. Tests pass a fake env and a fake tmux, so they never read the
// real environment or run tmux (ccx and ccproxy both cached detection in
// package variables, and ccproxy's cache made a later t.Setenv silently
// ineffective on CI). Every escape builder is a method on Caps, so whether a
// sequence is wrapped for tmux follows the value, not the process.
//
// # Nothing blocks Update or View
//
// tmux queries, image encoding and terminal writes run inside tea.Cmds and
// report a message with any error. View only ever returns placeholder text.
// ccx ran `tmux display-message` from Update every 250ms until it moved the
// poll into a command (sendbird/ccx#175); WatchPane is that command.
//
// # Placeholders, not cursor placement
//
// Image draws through Unicode placeholders (U+10EEEE): the image is
// transmitted once off the frame, and View returns ordinary text cells that
// the terminal paints the image over. Bubble Tea diffs those cells like any
// text, tmux stores them in the pane, so the image scrolls, clips and
// disappears with its window. A cursor-placed image (Place with MoveTo) floats
// at absolute coordinates instead: it survives a tmux window switch unless the
// app deletes it, which is what WatchPane and allow-passthrough=all are for.
package kitty
