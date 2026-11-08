package capture

import (
	"fmt"
	"image"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// darwinAPI resolves the CoreGraphics and CoreFoundation calls the
// capture needs, once.
var darwinAPI = sync.OnceValue(func() *darwinFuncs {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	f := &darwinFuncs{
		preflight:   mustSym(cg, "CGPreflightScreenCaptureAccess"),
		request:     mustSym(cg, "CGRequestScreenCaptureAccess"),
		listIDs:     mustSym(cg, "CGGetActiveDisplayList"),
		createImage: mustSym(cg, "CGDisplayCreateImage"),
		imgWidth:    mustSym(cg, "CGImageGetWidth"),
		imgHeight:   mustSym(cg, "CGImageGetHeight"),
		imgBPP:      mustSym(cg, "CGImageGetBitsPerPixel"),
		imgBPR:      mustSym(cg, "CGImageGetBytesPerRow"),
		imgBPC:      mustSym(cg, "CGImageGetBitsPerComponent"),
		imgInfo:     mustSym(cg, "CGImageGetBitmapInfo"),
		imgData:     mustSym(cg, "CGImageGetDataProvider"),
		provCopy:    mustSym(cg, "CGDataProviderCopyData"),
		dataLength:  mustSym(cf, "CFDataGetLength"),
		dataGet:     mustSym(cf, "CFDataGetBytes"),
		release:     mustSym(cf, "CFRelease"),
	}
	f.boundsSym = mustSym(cg, "CGDisplayBounds")
	purego.RegisterFunc(&f.displayBounds, f.boundsSym)
	return f
})

type darwinFuncs struct {
	preflight, request     uintptr
	listIDs                uintptr
	boundsSym              uintptr
	createImage            uintptr
	imgWidth, imgHeight    uintptr
	imgBPP, imgBPR, imgBPC uintptr
	imgInfo, imgData       uintptr
	provCopy               uintptr
	dataLength, dataGet    uintptr
	release                uintptr
	displayBounds          func(id uint32) nsRect
}

func mustSym(lib uintptr, name string) uintptr {
	fn, err := purego.Dlsym(lib, name)
	if err != nil {
		panic("capture: " + name + ": " + err.Error())
	}
	return fn
}

func call0(fn uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn)
	return r
}

func call1(fn uintptr, a uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a)
	return r
}

func preflight() bool {
	f := darwinAPI()
	return f != nil && call0(f.preflight) != 0
}

func request() bool {
	f := darwinAPI()
	return f != nil && call0(f.request) != 0
}

// nsRect is CGRect, returned by value; purego's RegisterFunc handles the
// struct return.
type nsRect struct{ X, Y, W, H float64 }

func displays() ([]Display, error) {
	f := darwinAPI()
	if f == nil {
		return nil, fmt.Errorf("capture: CoreGraphics did not load")
	}
	ids := make([]uint32, 32)
	count := uint32(len(ids))                                                                                                                    //nolint:gosec // G115: 32 ids fit uint32
	if res, _, _ := purego.SyscallN(f.listIDs, uintptr(len(ids)), uintptr(unsafe.Pointer(&ids[0])), uintptr(unsafe.Pointer(&count))); res != 0 { //nolint:gosec // G103/G115: purego's ABI takes pointers as uintptr
		return nil, fmt.Errorf("capture: CGGetActiveDisplayList: error %d", res)
	}
	count = min(count, uint32(len(ids))) //nolint:gosec // G115: 32 ids fit uint32
	out := make([]Display, 0, count)
	for i := range count {
		b := f.displayBounds(ids[i])
		out = append(out, Display{
			ID: ids[i],
			Bounds: image.Rect(
				int(b.X), int(b.Y),
				int(b.X+b.W), int(b.Y+b.H),
			),
		})
	}
	return out, nil
}

func captureDisplay(d Display) (*image.RGBA, error) {
	f := darwinAPI()
	if f == nil {
		return nil, fmt.Errorf("capture: CoreGraphics did not load")
	}
	img := call1(f.createImage, uintptr(d.ID))
	if img == 0 {
		return nil, fmt.Errorf("capture: CGDisplayCreateImage(%d) returned nil", d.ID)
	}
	defer call1(f.release, img)

	w, h := int(call1(f.imgWidth, img)), int(call1(f.imgHeight, img))
	bpp, bpr, bpc := int(call1(f.imgBPP, img)), int(call1(f.imgBPR, img)), int(call1(f.imgBPC, img))
	info := call1(f.imgInfo, img)
	prov := call1(f.imgData, img)
	if prov == 0 {
		return nil, fmt.Errorf("capture: image has no data provider")
	}
	data := call1(f.provCopy, prov)
	if data == 0 {
		return nil, fmt.Errorf("capture: could not copy image data")
	}
	defer call1(f.release, data)

	length := call1(f.dataLength, data)
	if length == 0 {
		return nil, fmt.Errorf("capture: image data is empty")
	}
	buf := make([]byte, int(length))
	// CFDataGetBytes(data, CFRange{0, length}, buffer): the range is a
	// pair of longs, which the C ABI passes in two registers.
	purego.SyscallN(f.dataGet, data, 0, length, uintptr(unsafe.Pointer(&buf[0]))) //nolint:gosec // G103: purego's ABI takes the buffer as uintptr; buf outlives the call

	return decode(w, h, bpp, bpr, bpc, uint32(info), buf) //nolint:gosec // G115: bitmapInfo is 32 bits of flags
}

// Bitmap info bits of CGImage.
const (
	alphaMask     = 0x0f
	byteOrderMask = 0x7000
	byteOrder32LE = 0x2000
	alphaFirst    = 0x02 // premultiplied alpha first: memory order BGRA
)

// decode turns the bytes of a CGImage into an RGBA image. Only the
// 32-bit BGRA premultiplied layout screenshots come in is supported.
func decode(w, h, bpp, bpr, bpc int, info uint32, src []byte) (*image.RGBA, error) {
	if bpp != 32 || bpc != 8 {
		return nil, fmt.Errorf("capture: unsupported pixel format %d bits/pixel", bpp)
	}
	if info&alphaMask != alphaFirst || info&byteOrderMask != byteOrder32LE {
		return nil, fmt.Errorf("capture: unsupported bitmap info 0x%x", info)
	}
	if len(src) < bpr*(h-1)+w*4 {
		return nil, fmt.Errorf("capture: image data too short: %d bytes for %dx%d", len(src), w, h)
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	dst := out.Pix
	for y := range h {
		row := y * bpr
		d := y * out.Stride
		for x := range w {
			s := row + x*4
			dst[d+0], dst[d+1], dst[d+2], dst[d+3] = src[s+2], src[s+1], src[s], src[s+3]
			d += 4
		}
	}
	return out, nil
}
