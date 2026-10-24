package server

// DRM dumb buffer 抓屏 (/dev/dri/card0): /screenshot 在 proc/mem 那条路失败时的退路。
// 原在 ai_glyph.go; 2026-10-24 AI 拆成独立应用, 抓屏留在空间 (应用找 /screenshot 要)。

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	drmIoctlBase             = 'd'
	drmIoctlModeGetResources = 0xC04064A0
	drmIoctlModeGetCrtc      = 0xC06864A1
	drmIoctlModeGetFB        = 0xC01464AD
	drmIoctlModeMapDumb      = 0xC01064B3
	drmIoctlModeGetFB2       = 0xC05464CE
)

type drmModeResources struct {
	FBIDPtr    uint64
	CRTCIDPtr  uint64
	ConnIDPtr  uint64
	EncIDPtr   uint64
	CountFBs   uint32
	CountCRTCs uint32
	CountConns uint32
	CountEncs  uint32
	MinW, MaxW uint32
	MinH, MaxH uint32
}

type drmModeCrtc struct {
	SetConnsPtr uint64
	CountConns  uint32
	CRTCID      uint32
	FBID        uint32
	X, Y        uint32
	GammaSize   uint32
	ModeValid   uint32
	Mode        [292]byte
}

type drmModeFBCmd struct {
	FBID   uint32
	Width  uint32
	Height uint32
	Pitch  uint32
	BPP    uint32
	Depth  uint32
	Handle uint32
}

type drmModeMapDumb struct {
	Handle uint32
	Pad    uint32
	Offset uint64
}

func drmIoctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func drmScreenshot() (image.Image, error) {
	f, err := os.OpenFile("/dev/dri/card0", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open drm: %w", err)
	}
	defer f.Close()
	fd := f.Fd()

	var res drmModeResources
	if err := drmIoctl(fd, drmIoctlModeGetResources, unsafe.Pointer(&res)); err != nil {
		return nil, fmt.Errorf("GETRESOURCES: %w", err)
	}
	if res.CountCRTCs == 0 {
		return nil, fmt.Errorf("no CRTCs")
	}

	crtcIDs := make([]uint32, res.CountCRTCs)
	fbIDs := make([]uint32, max32(res.CountFBs, 1))
	res.CRTCIDPtr = uint64(uintptr(unsafe.Pointer(&crtcIDs[0])))
	res.FBIDPtr = uint64(uintptr(unsafe.Pointer(&fbIDs[0])))
	connIDs := make([]uint32, max32(res.CountConns, 1))
	encIDs := make([]uint32, max32(res.CountEncs, 1))
	res.ConnIDPtr = uint64(uintptr(unsafe.Pointer(&connIDs[0])))
	res.EncIDPtr = uint64(uintptr(unsafe.Pointer(&encIDs[0])))
	if err := drmIoctl(fd, drmIoctlModeGetResources, unsafe.Pointer(&res)); err != nil {
		return nil, fmt.Errorf("GETRESOURCES2: %w", err)
	}

	crtc := drmModeCrtc{CRTCID: crtcIDs[0]}
	if err := drmIoctl(fd, drmIoctlModeGetCrtc, unsafe.Pointer(&crtc)); err != nil {
		return nil, fmt.Errorf("GETCRTC: %w", err)
	}
	if crtc.FBID == 0 {
		return nil, fmt.Errorf("no FB on CRTC")
	}

	fb := drmModeFBCmd{FBID: crtc.FBID}
	if err := drmIoctl(fd, drmIoctlModeGetFB, unsafe.Pointer(&fb)); err != nil {
		return nil, fmt.Errorf("GETFB: %w", err)
	}
	if fb.Width == 0 || fb.Height == 0 {
		return nil, fmt.Errorf("invalid FB size")
	}

	mapDumb := drmModeMapDumb{Handle: fb.Handle}
	if err := drmIoctl(fd, drmIoctlModeMapDumb, unsafe.Pointer(&mapDumb)); err != nil {
		return nil, fmt.Errorf("MAP_DUMB: %w", err)
	}

	size := int(fb.Pitch) * int(fb.Height)
	data, err := unix.Mmap(int(fd), int64(mapDumb.Offset), size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}
	defer unix.Munmap(data)

	bytesPerPixel := int(fb.BPP) / 8
	if bytesPerPixel < 1 {
		bytesPerPixel = 1
	}
	img := image.NewGray(image.Rect(0, 0, int(fb.Width), int(fb.Height)))
	for y := 0; y < int(fb.Height); y++ {
		for x := 0; x < int(fb.Width); x++ {
			off := y*int(fb.Pitch) + x*bytesPerPixel
			if off >= len(data) {
				break
			}
			var gray uint8
			switch bytesPerPixel {
			case 1:
				gray = data[off]
			case 2:
				// RGB565: extract green (middle bits) as approximation
				lo, hi := data[off], data[off+1]
				r5 := (hi >> 3) & 0x1F
				g6 := ((hi & 0x7) << 3) | (lo >> 5)
				b5 := lo & 0x1F
				gray = uint8((int(r5)*8 + int(g6)*4 + int(b5)*8) / 3)
			default:
				// xRGB / BGRX: channel[1] is reasonable for grayscale
				gray = data[off+1]
			}
			img.SetGray(x, y, color.Gray{Y: gray})
		}
	}
	return img, nil
}

func max32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
