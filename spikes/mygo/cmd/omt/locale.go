package main

import (
	"os"
	"strings"
)

// systemLocale reads the language the terminal is using. Chinese maps to
// the catalog's zh-Hans; anything else stays English, the source language.
func systemLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(key)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		return matchLocale(v)
	}
	return "en"
}

func matchLocale(v string) string {
	v = strings.ToLower(strings.SplitN(v, ".", 2)[0])
	v = strings.ReplaceAll(v, "_", "-")
	if strings.HasPrefix(v, "zh") {
		return "zh-Hans"
	}
	return "en"
}
