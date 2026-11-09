package shell

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// curlOmittedNote explains why a body is not in the command. It is a shell
// comment, so the command still runs as written.
const curlOmittedNote = "# body omitted: binary or truncated"

// curlBodyStdin is the flag that makes curl read the body from standard
// input, where the person pastes it. It stands in for bytes the command
// does not carry.
const curlBodyStdin = "--data-binary @-"

// curlCommand is the cURL command that sends the flow's request again. It
// keeps the method, the URL and the request headers. It drops the headers
// curl sets by itself, and carries the body only when it is text.
func curlCommand(f tool.Flow) string {
	method := f.Method
	if method == "" {
		method = http.MethodGet
	}
	bodyFlag, omitted := curlBody(f.Request.Body)
	args := []string{"curl"}
	if method != http.MethodGet || bodyFlag != "" {
		args = append(args, "-X", method)
	}
	args = append(args, shellQuote(f.URL))
	for _, h := range f.Request.Headers {
		if curlSkipsHeader(h.Name) {
			continue
		}
		args = append(args, "-H", shellQuote(h.Name+": "+h.Value))
	}
	if bodyFlag != "" {
		args = append(args, bodyFlag)
	}
	cmd := strings.Join(args, " ")
	if omitted {
		return curlOmittedNote + "\n" + cmd
	}
	return cmd
}

// curlSkipsHeader reports whether a request header is left out. Content-Length
// and Host are set by curl itself, and a name that starts with a colon is an
// HTTP/2 pseudo-header, which curl does not send.
func curlSkipsHeader(name string) bool {
	if name == "" || strings.HasPrefix(name, ":") {
		return true
	}
	switch strings.ToLower(name) {
	case "content-length", "host":
		return true
	}
	return false
}

// curlBody returns the flag that carries the body, and whether the body had
// to be left out. A body is binary when it was cut, is not valid UTF-8, or
// is of a media type that is not text.
func curlBody(b tool.Body) (flag string, omitted bool) {
	if len(b.Data) == 0 && !b.Truncated {
		return "", false
	}
	binary := b.Truncated || !utf8.Valid(b.Data) || (b.Type != "" && !isTextMedia(b.Type))
	if binary {
		return curlBodyStdin, true
	}
	return "--data-raw " + shellQuote(string(b.Data)), false
}

// shellQuote quotes s as one POSIX shell word. An embedded single quote is
// written as close quote, escaped quote, reopen quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
