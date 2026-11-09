package shell

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Size units, by powers of 1024.
const (
	kilobyte = 1 << 10
	megabyte = 1 << 20
	gigabyte = 1 << 30
)

// trafficSize writes a number of bytes in the language shown.
func trafficSize(n int64) string {
	switch {
	case n >= gigabyte:
		return i18n.SizeGB(float64(n) / gigabyte)
	case n >= megabyte:
		return i18n.SizeMB(float64(n) / megabyte)
	case n >= kilobyte:
		return i18n.SizeKB(float64(n) / kilobyte)
	}
	return i18n.SizeBytes(int(n))
}

// trafficTook writes how long a flow took: milliseconds while it is
// quick, seconds with a tenth once it is not.
func trafficTook(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d < time.Second:
		return i18n.TrafficMillis(int(d.Milliseconds()))
	}
	return i18n.TrafficSeconds(d.Seconds())
}

// shortType writes a media type the way a dense log has room for: the
// subtype, with the few names that read badly spelled out.
func shortType(media string) string {
	if media == "" {
		return ""
	}
	_, sub, ok := strings.Cut(media, "/")
	if !ok {
		return media
	}
	switch sub {
	case "x-www-form-urlencoded":
		return "form"
	case "octet-stream":
		return "bin"
	case "javascript":
		return "js"
	case "plain":
		return "text"
	}
	return sub
}

// trafficRowLabel is the accessible name of a row: what a person would
// read of it.
func trafficRowLabel(r tool.FlowRow) string {
	if r.Source == tool.SourcePacket {
		return strings.Join([]string{"#" + fmt.Sprint(r.ID), r.Proto, r.Host, r.Info}, " ")
	}
	line := fmt.Sprintf("#%d %s %s%s", r.ID, r.Method, r.Host, r.Path)
	if r.Status > 0 {
		line += " " + fmt.Sprint(r.Status)
	}
	return line + " " + r.Type
}

// listSelection is the fill of a chosen row: the design system's
// selection token over the card, so that a row reads as chosen in either
// theme while the list's own accent mark, and the text it would pale, are
// covered. PassThrough lets the pointer reach the row itself.
func listSelection(c *ui.Context) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ds.P(c).Selection).PassThrough()
}

// statusTone is the colour of a response status: a success, a redirect, a
// refusal by the server, and a fault.
func statusTone(status int) comp.Tone {
	switch {
	case status >= 500:
		return comp.ToneDanger
	case status >= 400:
		return comp.ToneWarning
	case status >= 300:
		return comp.ToneAccent
	case status >= 200:
		return comp.ToneSuccess
	}
	return comp.ToneNeutral
}
