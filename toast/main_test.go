package toast

import (
	"os"
	"testing"
)

// TestMain pins a non-CJK UTF-8 locale so the default Ellipsis is `…`.
func TestMain(m *testing.M) {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "RUNEWIDTH_EASTASIAN"} {
		os.Unsetenv(k)
	}
	os.Setenv("LANG", "en_US.UTF-8")
	os.Exit(m.Run())
}
