package main

import "testing"

func TestMatchLocale(t *testing.T) {
	got := map[string]string{
		"zh_CN.UTF-8": matchLocale("zh_CN.UTF-8"),
		"zh-Hans":     matchLocale("zh-Hans"),
		"en_US.UTF-8": matchLocale("en_US.UTF-8"),
		"fr_FR":       matchLocale("fr_FR"),
	}
	want := map[string]string{
		"zh_CN.UTF-8": "zh-Hans",
		"zh-Hans":     "zh-Hans",
		"en_US.UTF-8": "en",
		"fr_FR":       "en",
	}
	for in, loc := range got {
		if loc != want[in] {
			t.Errorf("%s: %q, want %q", in, loc, want[in])
		}
	}
}
