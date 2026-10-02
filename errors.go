package serpkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Error is returned for every failed call. API errors carry the fields of the
// {"error":{"code","message","request_id"}} envelope. Network failures and
// timeouts have Status 0 and Code "connection_error" or "timeout".
type Error struct {
	// Status is the HTTP status, or 0 when no response was received (and for
	// entries rejected by Batches.Create).
	Status int
	// Code is machine-readable, e.g. unauthorized, invalid_request,
	// insufficient_credits, rate_limited, upstream_error, spend_cap_reached.
	Code      string
	Message   string
	RequestID string
	// Header holds the response headers, when there was a response.
	Header http.Header

	cause error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("serpkite: ")
	if e.Status != 0 {
		fmt.Fprintf(&b, "%d ", e.Status)
	}
	b.WriteString(e.Code)
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request_id %s)", e.RequestID)
	}
	return b.String()
}

// Unwrap returns the underlying transport error, if any.
func (e *Error) Unwrap() error { return e.cause }

// Retryable reports whether the failure is a rate limit, server error or network error.
func (e *Error) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500 || e.Code == "connection_error" || e.Code == "timeout"
}

var fallbackCodes = map[int]string{
	400: "invalid_request",
	401: "unauthorized",
	402: "insufficient_credits",
	403: "forbidden",
	404: "not_found",
	429: "rate_limited",
	// The API sends upstream failures as 503 with a JSON body; a bare 502/504
	// comes from a proxy (Cloudflare replaces origin 502/504 bodies).
	502: "upstream_error",
	503: "unavailable",
	504: "upstream_timeout",
}

func errorFromResponse(status int, h http.Header, body []byte) *Error {
	e := &Error{Status: status, Header: h, RequestID: h.Get("X-Request-Id")}
	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil && env.Error.Code != "" {
		e.Code, e.Message = env.Error.Code, env.Error.Message
		if env.Error.RequestID != "" {
			e.RequestID = env.Error.RequestID
		}
		return e
	}
	e.Code = fallbackCodes[status]
	if e.Code == "" {
		e.Code = "http_error"
		if status >= 500 {
			e.Code = "server_error"
		}
	}
	e.Message = strings.TrimSpace(string(body))
	if len(e.Message) > 500 {
		e.Message = e.Message[:500]
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}
