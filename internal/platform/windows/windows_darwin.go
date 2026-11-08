package windows

import (
	"fmt"
	"image"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// CGWindowListOption bits and the window ID the listing is taken with:
// only windows on screen, without the desktop picture and icons, relative
// to no particular window.
const (
	listOnScreenOnly           = 1 << 0 // kCGWindowListOptionOnScreenOnly
	listExcludeDesktopElements = 1 << 4 // kCGWindowListExcludeDesktopElements
	nullWindowID               = 0      // kCGNullWindowID
)

const (
	// normalLayer is the kCGWindowLayer of ordinary application windows;
	// the menu bar, the Dock and floating panels sit on other layers.
	normalLayer = 0
	// numberSInt32 is kCFNumberSInt32Type.
	numberSInt32 = 3
	// minSide is the smallest width and height, in points, a window must
	// have to be offered: anything smaller is a helper, not a target.
	minSide = 40
)

// cgRect is CGRect, filled by CoreGraphics; purego's RegisterFunc handles
// the struct.
type cgRect struct{ X, Y, W, H float64 }

// darwinFuncs holds the CoreGraphics and CoreFoundation bindings the
// listing needs.
type darwinFuncs struct {
	copyInfo   func(option, window uint32) uintptr
	rectFrom   func(dict uintptr, rect *cgRect) bool
	arrayCount func(arr uintptr) int
	arrayAt    func(arr uintptr, i int) uintptr
	dictGet    func(dict, key uintptr) uintptr
	numberGet  func(num uintptr, kind int, out *int32) bool
	release    func(obj uintptr)

	boundsKey, layerKey uintptr
}

// darwinAPI resolves the bindings once; it is nil when a framework did not
// load.
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
		boundsKey: constant(cg, "kCGWindowBounds"),
		layerKey:  constant(cg, "kCGWindowLayer"),
	}
	purego.RegisterLibFunc(&f.copyInfo, cg, "CGWindowListCopyWindowInfo")
	purego.RegisterLibFunc(&f.rectFrom, cg, "CGRectMakeWithDictionaryRepresentation")
	purego.RegisterLibFunc(&f.arrayCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&f.arrayAt, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&f.dictGet, cf, "CFDictionaryGetValue")
	purego.RegisterLibFunc(&f.numberGet, cf, "CFNumberGetValue")
	purego.RegisterLibFunc(&f.release, cf, "CFRelease")
	return f
})

// constant reads an exported CFStringRef constant such as kCGWindowBounds:
// Dlsym gives the address of the variable, which holds the reference.
func constant(lib uintptr, name string) uintptr {
	addr, err := purego.Dlsym(lib, name)
	if err != nil {
		panic("windows: " + name + ": " + err.Error())
	}
	return **(**uintptr)(unsafe.Pointer(&addr)) //nolint:gosec // G103: addr is the address of a CFStringRef variable exported by CoreGraphics
}

func list() ([]image.Rectangle, error) {
	f := darwinAPI()
	if f == nil {
		return nil, fmt.Errorf("windows: CoreGraphics did not load")
	}
	arr := f.copyInfo(listOnScreenOnly|listExcludeDesktopElements, nullWindowID)
	if arr == 0 {
		return nil, fmt.Errorf("windows: CGWindowListCopyWindowInfo returned nil")
	}
	defer f.release(arr)

	count := f.arrayCount(arr)
	out := make([]image.Rectangle, 0, count)
	for i := range count {
		frame, ok, err := f.frame(f.arrayAt(arr, i))
		if err != nil {
			return nil, fmt.Errorf("windows: window %d: %w", i, err)
		}
		if ok {
			out = append(out, frame)
		}
	}
	return out, nil
}

// frame reads the frame of one window info dictionary. ok is false when
// the window is not one to offer: not on the normal layer, or too small.
func (f *darwinFuncs) frame(info uintptr) (frame image.Rectangle, ok bool, err error) {
	if layer, has := f.layer(info); has && layer != normalLayer {
		return image.Rectangle{}, false, nil
	}
	bounds := f.dictGet(info, f.boundsKey)
	var r cgRect
	if bounds == 0 || !f.rectFrom(bounds, &r) {
		return image.Rectangle{}, false, fmt.Errorf("no readable bounds")
	}
	if r.W < minSide || r.H < minSide {
		return image.Rectangle{}, false, nil
	}
	return image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H)), true, nil
}

// layer reads kCGWindowLayer of a window info dictionary.
func (f *darwinFuncs) layer(info uintptr) (int32, bool) {
	num := f.dictGet(info, f.layerKey)
	if num == 0 {
		return 0, false
	}
	var layer int32
	if !f.numberGet(num, numberSInt32, &layer) {
		return 0, false
	}
	return layer, true
}
