// Package rules applies the rule book of the traffic tool to the requests
// and responses that pass through its proxy.
package rules

import (
	"net/http"
	"time"
)

// Verdict is what the rules decided about a request.
type Verdict struct {
	// Block closes the connection, with no answer.
	Block bool
	// Mock answers with this instead of sending the request on; its body is
	// fully buffered.
	Mock *http.Response
	// Delay holds the response back this long.
	Delay time.Duration
	// Hits names the rules that applied.
	Hits []string
}
