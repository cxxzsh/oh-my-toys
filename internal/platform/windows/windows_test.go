//go:build darwin

package windows

import "testing"

// sampleFrames is how many frames the test prints to show real numbers.
const sampleFrames = 3

func TestListFramesAreNonEmpty(t *testing.T) {
	frames, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Skip("no on-screen windows")
	}
	t.Logf("%d on-screen windows", len(frames))
	for i, r := range frames {
		if r.Empty() || r.Dx() < minSide || r.Dy() < minSide {
			t.Fatalf("window %d has frame %v", i, r)
		}
		if i < sampleFrames {
			t.Logf("window %d: %v (%dx%d)", i, r, r.Dx(), r.Dy())
		}
	}
}
