package color

import (
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	want := RGB{0, 113, 227}
	for _, in := range []string{"#0071e3", "0071e3", "rgb(0, 113, 227)", "0,113,227", "rgba(0 113 227 / 50%)"} {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if got, _ := Parse("#fff"); got != (RGB{255, 255, 255}) {
		t.Errorf("#fff: %v", got)
	}
	if got, _ := Parse("hsl(0, 100%, 50%)"); got != (RGB{255, 0, 0}) {
		t.Errorf("hsl red: %v", got)
	}
	for _, bad := range []string{"", "blue-ish", "rgb(300, 0, 0)", "#12"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestContrast(t *testing.T) {
	if r := Contrast(RGB{}, RGB{255, 255, 255}); math.Abs(r-21) > 0.01 {
		t.Errorf("black on white %.2f", r)
	}
	h, s, l := RGB{0, 113, 227}.HSL()
	if back := FromHSL(h, s, l); back != (RGB{0, 113, 227}) {
		t.Errorf("round trip %v", back)
	}
}
