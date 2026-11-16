//go:build !linux

package server

import (
	"errors"
	"image"
)

func drmScreenshot() (image.Image, error) {
	return nil, errors.New("DRM screenshots are only supported on Linux")
}
