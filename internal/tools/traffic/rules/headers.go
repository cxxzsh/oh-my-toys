package rules

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// headerHost is how a rule names the request's Host header.
const headerHost = "Host"

// checkHeaderEdits rejects an edit a writer could not put on the wire. It
// runs at Set time, so a value carrying a line break can never reach a raw
// writer and split the message.
func checkHeaderEdits(i int, r tool.Rule, edits []tool.HeaderEdit) error {
	for _, e := range edits {
		if err := checkHeaderName(i, r, e.Name); err != nil {
			return err
		}
		if e.Remove {
			continue
		}
		if err := checkHeaderValue(i, r, e.Name, e.Value); err != nil {
			return err
		}
	}
	return nil
}

// checkMockHeaders rejects a mock header a writer could not put on the wire.
func checkMockHeaders(i int, r tool.Rule, headers []tool.Header) error {
	for _, h := range headers {
		if err := checkHeaderName(i, r, h.Name); err != nil {
			return err
		}
		if err := checkHeaderValue(i, r, h.Name, h.Value); err != nil {
			return err
		}
	}
	return nil
}

// checkHeaderName rejects a name that is not an HTTP field name.
func checkHeaderName(i int, r tool.Rule, name string) error {
	if !httpguts.ValidHeaderFieldName(name) {
		return invalidRule(i, r, fmt.Sprintf("header name %q is not a valid field name", name))
	}
	return nil
}

// checkHeaderValue rejects a value with a character that may not be sent.
func checkHeaderValue(i int, r tool.Rule, name, value string) error {
	if !httpguts.ValidHeaderFieldValue(value) {
		return invalidRule(i, r, fmt.Sprintf("value of header %q holds a character that may not be sent", name))
	}
	return nil
}

// editHeaders sets or removes headers, whatever case the rule used and
// whatever case the message holds the header in. Set has already checked
// the names, so a name here is a valid field name.
func editHeaders(h http.Header, edits []tool.HeaderEdit) {
	for _, e := range edits {
		name := http.CanonicalHeaderKey(e.Name)
		removeHeader(h, name)
		if !e.Remove {
			h[name] = []string{e.Value}
		}
	}
}

// removeHeader deletes a header under every spelling h holds it in, since a
// message built by hand may not use the canonical case.
func removeHeader(h http.Header, name string) {
	delete(h, name)
	for key := range h {
		if strings.EqualFold(key, name) {
			delete(h, key)
		}
	}
}

// editRequestHeaders applies header edits to an outgoing request. Host is
// not kept in the header map: the request sends req.Host, so the edit has
// to go there or it would change nothing.
func editRequestHeaders(req *http.Request, edits []tool.HeaderEdit) {
	rest := make([]tool.HeaderEdit, 0, len(edits))
	for _, e := range edits {
		if !strings.EqualFold(e.Name, headerHost) {
			rest = append(rest, e)
			continue
		}
		if e.Remove {
			req.Host = ""
		} else {
			req.Host = e.Value
		}
	}
	editHeaders(req.Header, rest)
}
