// Package har writes what the traffic tool's proxy recorded as HAR 1.2, the
// format the browsers' developer tools read.
package har

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

const (
	// harVersion is the version of the format written.
	harVersion = "1.2"
	// creatorName is the program the file says it came from.
	creatorName = "oh-my-toys"
	// creatorVersion is the version in the creator block. The app stamps
	// no build version below the core, so a development build says so.
	creatorVersion = "dev"
	// unknownSize is what HAR writes for a size or a phase that was not
	// measured.
	unknownSize = -1
	// base64Encoding marks content that is bytes, not text.
	base64Encoding = "base64"

	headFormat = `{"log":{"version":%q,"creator":%s,"entries":[`
	trailer    = "]}}"
	separator  = ","
)

// creator is who wrote the document.
type creator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Write writes the proxied flows of flows as HAR 1.2, streaming one entry at
// a time. Packets are skipped, and so is a CONNECT tunnel that carries no
// content: neither holds anything the format can show.
func Write(w io.Writer, flows []tool.Flow) error {
	if err := writeHead(w); err != nil {
		return err
	}
	first := true
	for i := range flows {
		if skippable(&flows[i]) {
			continue
		}
		if !first {
			if _, err := io.WriteString(w, separator); err != nil {
				return fmt.Errorf("write har separator: %w", err)
			}
		}
		first = false
		if err := writeEntry(w, &flows[i]); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, trailer); err != nil {
		return fmt.Errorf("write har end: %w", err)
	}
	return nil
}

func writeHead(w io.Writer) error {
	block, err := json.Marshal(creator{Name: creatorName, Version: creatorVersion})
	if err != nil {
		return fmt.Errorf("encode har creator: %w", err)
	}
	if _, err := fmt.Fprintf(w, headFormat, harVersion, block); err != nil {
		return fmt.Errorf("write har head: %w", err)
	}
	return nil
}

func writeEntry(w io.Writer, f *tool.Flow) error {
	data, err := json.Marshal(entryFor(f))
	if err != nil {
		return fmt.Errorf("encode har entry %d: %w", f.ID, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write har entry %d: %w", f.ID, err)
	}
	return nil
}

// skippable reports whether a flow has nothing a HAR entry can show: a
// packet, or a tunnel that was never opened.
func skippable(f *tool.Flow) bool {
	if f.Source == tool.SourcePacket {
		return true
	}
	if !strings.EqualFold(f.Method, http.MethodConnect) {
		return false
	}
	return len(f.Request.Body.Data) == 0 && len(f.Response.Body.Data) == 0
}
