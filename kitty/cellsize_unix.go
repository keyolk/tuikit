//go:build unix

package kitty

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func cellSizeFromFD(fd uintptr) (CellSize, error) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil {
		return CellSize{}, fmt.Errorf("kitty: window size: %w", err)
	}
	if ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return CellSize{}, errors.New("kitty: terminal reports no pixel size")
	}
	return CellSize{W: int(ws.Xpixel) / int(ws.Col), H: int(ws.Ypixel) / int(ws.Row)}.OrDefault(), nil
}
