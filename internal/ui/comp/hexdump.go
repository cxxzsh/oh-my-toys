package comp

import (
	"fmt"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// hexOffsetDigits is the width of a row's address in a hex dump: enough
// for the 4 MiB a body keeps.
const hexOffsetDigits = 8

// printableFirst and printableLast bound the characters a dump shows as
// themselves; anything else is a dot.
const (
	printableFirst = 0x20
	printableLast  = 0x7e
)

// HexDump writes data the way a protocol analyser shows bytes: the
// address of every row, the ds.HexRow bytes in two groups of ds.HexGroup,
// and at the end the bytes that are printable, a dot for the rest:
//
//	00000000  48 54 54 50 2f 31 2e 31  20 32 30 30 20 4f 4b 0d  |HTTP/1.1 200 OK.|
//	00000010  0a                                               |.|
//
// It is pure, so a view may call it while drawing.
func HexDump(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var b strings.Builder
	rows := (len(data) + ds.HexRow - 1) / ds.HexRow
	b.Grow(rows * (2*hexOffsetDigits + 4 + ds.HexRow*4))
	for start := 0; start < len(data); start += ds.HexRow {
		if start > 0 {
			b.WriteByte('\n')
		}
		hexRow(&b, data[start:min(start+ds.HexRow, len(data))], start)
	}
	return b.String()
}

// hexRow writes one line: the address, the bytes, then their characters.
func hexRow(b *strings.Builder, row []byte, at int) {
	fmt.Fprintf(b, "%0*x  ", hexOffsetDigits, at)
	for i := range ds.HexRow {
		if i == ds.HexGroup {
			b.WriteByte(' ')
		}
		switch {
		case i < len(row):
			fmt.Fprintf(b, "%02x ", row[i])
		default:
			b.WriteString("   ")
		}
	}
	b.WriteString(" |")
	for _, c := range row {
		if c >= printableFirst && c <= printableLast {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('.')
	}
	b.WriteByte('|')
}
