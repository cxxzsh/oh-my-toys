package har

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// startedLayout is RFC 3339 with the milliseconds HAR shows.
const startedLayout = "2006-01-02T15:04:05.000Z07:00"

// entryFor turns one flow into the entry HAR shows of it.
func entryFor(f *tool.Flow) entry {
	waited := millis(f.Duration)
	return entry{
		Started:  startedAt(f.Started),
		Time:     waited,
		Request:  requestFor(f),
		Response: responseFor(f),
		Timings:  timings{Wait: waited},
	}
}

func requestFor(f *tool.Flow) request {
	return request{
		Method:      f.Method,
		URL:         f.URL,
		HTTPVersion: f.Proto,
		Cookies:     requestCookies(f.Request.Headers),
		Headers:     headersOf(f.Request.Headers),
		QueryString: queryOf(f.URL),
		PostData:    postDataOf(f.Request),
		HeadersSize: unknownSize,
		BodySize:    f.Request.Body.Size,
	}
}

func responseFor(f *tool.Flow) response {
	return response{
		Status:      f.Status,
		StatusText:  http.StatusText(f.Status),
		HTTPVersion: f.Proto,
		Cookies:     responseCookies(f.Response.Headers),
		Headers:     headersOf(f.Response.Headers),
		Content:     contentOf(f.Response.Body),
		RedirectURL: redirectOf(f.Response.Headers),
		HeadersSize: unknownSize,
		BodySize:    f.Response.Body.Size,
	}
}

// headersOf keeps the header lines in the order and the case they were
// sent in.
func headersOf(in []tool.Header) []header {
	out := make([]header, len(in))
	for i, h := range in {
		out[i] = header{Name: h.Name, Value: h.Value}
	}
	return out
}

// queryOf splits the query of rawURL as it was sent, escapes decoded.
func queryOf(rawURL string) []parameter {
	pairs := make([]parameter, 0)
	u, err := url.Parse(rawURL)
	if err != nil {
		return pairs
	}
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		pairs = append(pairs, parameter{Name: unescape(name), Value: unescape(value)})
	}
	return pairs
}

func unescape(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// requestCookies reads the Cookie headers, of which a request may have
// several.
func requestCookies(in []tool.Header) []cookie {
	out := make([]cookie, 0)
	for _, h := range in {
		if !strings.EqualFold(h.Name, "Cookie") {
			continue
		}
		out = append(out, cookiePairs(h.Value)...)
	}
	return out
}

func cookiePairs(value string) []cookie {
	out := make([]cookie, 0)
	for _, pair := range strings.Split(value, ";") {
		name, val, ok := strings.Cut(strings.TrimSpace(pair), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		out = append(out, cookie{Name: name, Value: val})
	}
	return out
}

// responseCookies reads one cookie from each Set-Cookie header.
func responseCookies(in []tool.Header) []cookie {
	out := make([]cookie, 0)
	for _, h := range in {
		if !strings.EqualFold(h.Name, "Set-Cookie") {
			continue
		}
		c, err := http.ParseSetCookie(h.Value)
		if err != nil || c.Name == "" {
			continue
		}
		out = append(out, setCookie(c))
	}
	return out
}

func setCookie(c *http.Cookie) cookie {
	out := cookie{
		Name:     c.Name,
		Value:    c.Value,
		Path:     c.Path,
		Domain:   c.Domain,
		HTTPOnly: c.HttpOnly,
		Secure:   c.Secure,
	}
	if !c.Expires.IsZero() {
		out.Expires = c.Expires.UTC().Format(time.RFC3339)
	}
	return out
}

// contentOf writes the response body as text when it is text and as base64
// bytes otherwise, which is how HAR carries what is not text.
func contentOf(b tool.Body) content {
	out := content{Size: int64(len(b.Data)), MimeType: b.Type}
	if len(b.Data) == 0 {
		return out
	}
	if isTextType(b.Type) {
		out.Text = string(b.Data)
		return out
	}
	out.Text = base64.StdEncoding.EncodeToString(b.Data)
	out.Encoding = base64Encoding
	return out
}

// postDataOf writes the request body when it is text; HAR cannot show a
// binary one.
func postDataOf(m tool.HTTPMessage) *postData {
	if len(m.Body.Data) == 0 || !isTextType(m.Body.Type) {
		return nil
	}
	return &postData{MimeType: m.Body.Type, Text: string(m.Body.Data)}
}

func redirectOf(in []tool.Header) string {
	for _, h := range in {
		if strings.EqualFold(h.Name, "Location") {
			return h.Value
		}
	}
	return ""
}

func startedAt(t time.Time) string {
	return t.Format(startedLayout)
}

func millis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// isTextType reports whether a body of media type t holds text. It is the
// same rule the flow log searches by: a body with a type it does not know
// is bytes.
func isTextType(t string) bool {
	t = strings.ToLower(t)
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	if strings.HasPrefix(t, "text/") || strings.HasSuffix(t, "+json") || strings.HasSuffix(t, "+xml") {
		return true
	}
	switch t {
	case "application/json", "application/xml",
		"application/x-www-form-urlencoded", "application/javascript":
		return true
	}
	return false
}
