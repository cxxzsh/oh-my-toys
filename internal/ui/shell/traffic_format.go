package shell

import (
	"fmt"
	"strconv"
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

// rowText is the text a row shows, made once for a flow and kept while the
// flow reads the same. Rows in view are drawn every frame, and formatting
// them each time would cost the frame.
type rowText struct {
	key                                 flowKey
	label, id, time, size, took, status string
}

// flowKey is what a row's text is made from. A flow still arriving moves
// these, and only these, so the row is made again when one of them moves.
type flowKey struct {
	status int
	state  tool.FlowState
	size   int64
	took   time.Duration
	typ    string
	info   string
}

// rowTextSlack is how many cached rows may exceed the rows listed before
// the cache is emptied, so that it cannot grow without bound.
const rowTextSlack = 64

// textOf is the text of a row, made now if the cache has none for the flow
// or the flow has changed since.
func (st *trafficState) textOf(r tool.FlowRow) rowText {
	key := flowKey{r.Status, r.State, r.Size, r.Duration, r.Type, r.Info}
	if txt, ok := st.texts[r.ID]; ok && txt.key == key {
		return txt
	}
	if st.texts == nil || len(st.texts) > 2*len(st.rows)+rowTextSlack {
		st.texts = make(map[uint64]rowText)
	}
	txt := rowText{
		key:    key,
		label:  trafficRowLabel(r),
		id:     strconv.FormatUint(r.ID, 10),
		time:   r.Started.Format(time.TimeOnly),
		size:   trafficSize(r.Size),
		took:   trafficTook(r.Duration),
		status: rowStatusText(r),
	}
	st.texts[r.ID] = txt
	return txt
}

// rowStatusText is the word a flow's list row shows as its status: its status
// code, or the word for a block, a failure, or a response still on its way.
func rowStatusText(r tool.FlowRow) string {
	switch r.State {
	case tool.FlowBlocked:
		return i18n.TrafficBlocked()
	case tool.FlowFailed:
		return i18n.TrafficFailed()
	case tool.FlowOpen:
		if r.Status == 0 {
			return i18n.TrafficOpen()
		}
	case tool.FlowDone:
	}
	return strconv.Itoa(r.Status)
}
