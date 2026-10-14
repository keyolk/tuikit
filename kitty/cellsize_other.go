//go:build !unix

package kitty

import "errors"

func cellSizeFromFD(uintptr) (CellSize, error) {
	return CellSize{}, errors.New("kitty: pixel size needs a unix terminal")
}
