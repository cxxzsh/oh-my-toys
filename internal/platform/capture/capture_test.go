//go:build darwin

package capture

import "testing"

// bpp32LEFirst is what CGImageGetBitmapInfo reports for the BGRA
// premultiplied screenshots decode handles: 32-bit little-endian pixels,
// alpha first in memory.
const bpp32LEFirst = byteOrder32LE | alphaFirst

func TestDecodeBGRA(t *testing.T) {
	// Two pixels side by side, 8 bytes per row padded to 16.
	row := []byte{
		0x11, 0x22, 0x33, 0xff, // B G R A -> R=0x33 G=0x22 B=0x11
		0x44, 0x55, 0x66, 0x80, // B G R A -> R=0x66 G=0x55 B=0x44
		0, 0, 0, 0, 0, 0, 0, 0, // padding to bytesPerRow 16
	}
	img, err := decode(2, 1, 32, 16, 8, bpp32LEFirst, row)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x33, 0x22, 0x11, 0xff, 0x66, 0x55, 0x44, 0x80}
	for i, b := range want {
		if img.Pix[i] != b {
			t.Fatalf("pixel byte %d = %#x, want %#x (whole row %v)", i, img.Pix[i], b, img.Pix[:8])
		}
	}
}

func TestDecodeRejectsOtherFormats(t *testing.T) {
	src := make([]byte, 64)
	for _, c := range []struct {
		name     string
		w, h     int
		bpp, bpc int
		info     uint32
		src, bpr int
	}{
		{"24 bits", 2, 1, 24, 8, bpp32LEFirst, 16, 16},
		{"not BGRA order", 2, 1, 32, 8, byteOrder32LE | 0x01, 16, 16},
		{"data too short", 2, 2, 32, 8, bpp32LEFirst, 16, 16},
	} {
		if _, err := decode(c.w, c.h, c.bpp, c.bpr, c.bpc, c.info, src[:c.src]); err == nil {
			t.Errorf("%s: decoded without error", c.name)
		}
	}
}
