package selection

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// CopiedMsg reports the result of Copy.
type CopiedMsg struct {
	Text string
	Err  error
}

// Note is the toast text for the copy: "copied 12 chars" or the error.
func (m CopiedMsg) Note() string { return m.NoteWith("copied %d chars", "copy failed: %s") }

// NoteWith is Note in other words: copied gets the character count (%d),
// failed the error (%s). A Korean UI passes "복사: %d자", "복사 실패: %s".
func (m CopiedMsg) NoteWith(copied, failed string) string {
	if m.Err != nil {
		return fmt.Sprintf(failed, m.Err.Error())
	}
	return fmt.Sprintf(copied, m.Chars())
}

// Chars is how many characters were copied.
func (m CopiedMsg) Chars() int { return utf8.RuneCountInString(m.Text) }

// Clipboard writes text to the clipboard; Copy calls it. Tests replace it
// so that running the command Update returned is safe and the copied text
// can be asserted from the CopiedMsg, the same path the app takes.
var Clipboard = writeClipboard

// Copy puts text on the clipboard off the UI thread. Locally it uses the
// platform tool (pbcopy, wl-copy, xclip), which never writes to the terminal
// the renderer is drawing on. Over SSH, or when no tool exists, it falls back
// to OSC 52 on /dev/tty, as the engine does; tmux forwards it with
// set-clipboard on.
func Copy(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		return CopiedMsg{Text: text, Err: Clipboard(text)}
	}
}

var clipboardTools = map[string][][]string{
	"darwin": {{"pbcopy"}},
	"linux":  {{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}},
}

func writeClipboard(text string) error {
	if os.Getenv("SSH_TTY") == "" && os.Getenv("SSH_CONNECTION") == "" {
		for _, argv := range clipboardTools[runtime.GOOS] {
			if _, err := exec.LookPath(argv[0]); err != nil {
				continue
			}
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	_, err = tty.WriteString(OSC52(text))
	return err
}

// OSC52 is the clipboard-write escape for text.
func OSC52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}
