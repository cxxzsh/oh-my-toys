package har

// The document written: only the fields HAR 1.2 requires of an entry, and
// nothing invented. The types stay unexported because a flow of the traffic
// tool is the only input the package takes.

type entry struct {
	Started  string   `json:"startedDateTime"`
	Time     float64  `json:"time"`
	Request  request  `json:"request"`
	Response response `json:"response"`
	Cache    cache    `json:"cache"`
	Timings  timings  `json:"timings"`
}

// cache is the empty cache HAR requires of every entry.
type cache struct{}

// timings is how the entry's time was spent. Only the total is known, so
// the phases that were not measured are 0 and wait carries the total.
type timings struct {
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

type request struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Cookies     []cookie    `json:"cookies"`
	Headers     []header    `json:"headers"`
	QueryString []parameter `json:"queryString"`
	PostData    *postData   `json:"postData,omitempty"`
	HeadersSize int         `json:"headersSize"`
	BodySize    int64       `json:"bodySize"`
}

type response struct {
	Status      int      `json:"status"`
	StatusText  string   `json:"statusText"`
	HTTPVersion string   `json:"httpVersion"`
	Cookies     []cookie `json:"cookies"`
	Headers     []header `json:"headers"`
	Content     content  `json:"content"`
	RedirectURL string   `json:"redirectURL"`
	HeadersSize int      `json:"headersSize"`
	BodySize    int64    `json:"bodySize"`
}

type header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// cookie is one cookie of a request or a response. The fields HAR allows to
// be left out stay out when the flow does not know them.
type cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
}

type content struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// postData is the text request body. HAR has no place for a binary one, so
// a request without text has none.
type postData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}
