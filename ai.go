package serpkite

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ── Answer ───────────────────────────────────────────────────────────────

// Answer returns a cited answer grounded on live Google results: Markdown
// with "[n]" markers in Answer and the cited sources in Citations.
// DepthFast answers from the result page; DepthStandard (default) also reads
// the top NumSources pages. With OutputSchema the answer is a JSON value in
// Output. A failed model call is returned as an *Error with code
// "ai_unavailable" (503) and the model part is refunded; a search with
// nothing to answer from returns a nil Answer and a Meta.Warnings entry.
//
// Answer is billed even when the response is lost (and an ai_unavailable
// error still charges the search part), so it is retried only on 429, and
// its per-attempt timeout is at least 100 s: the search, pages read included,
// runs before the model writes.
func (c *Client) Answer(ctx context.Context, p AnswerParams, opts ...RequestOption) (*AnswerResponse, error) {
	p.Stream = false
	var out AnswerResponse
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: "/v1/answer", body: p, opts: opts, out: &out, rateLimitOnly: true, minTimeout: answerMinTimeout}); err != nil {
		return nil, err
	}
	return &out, nil
}

// answerMinTimeout is the least per-attempt timeout of an answer: the search
// runs before the first byte (the API answers within Cloudflare's 100 s).
const answerMinTimeout = 100 * time.Second

// AnswerStream streams an answer as Server-Sent Events: one "results" event,
// "delta" events with the text as it is written, then "done". Close the
// stream when finished.
//
//	s, err := c.AnswerStream(ctx, serpkite.AnswerParams{Q: "what is retrieval-augmented generation"})
//	if err != nil { return err }
//	defer s.Close()
//	for s.Next() {
//		if ev := s.Event(); ev.Delta != nil { fmt.Print(ev.Delta.Text) }
//	}
//	if err := s.Err(); err != nil { return err }
//
// Errors before the stream starts (400, 402, 429, …) are returned by
// AnswerStream itself; 429 and 5xx are retried like other calls (nothing is
// charged before the stream starts). OutputSchema can't be streamed. The
// per-attempt timeout ([WithTimeout], at least 100 s since the search runs
// first) covers the wait for the response headers only; bound the whole
// stream with ctx.
func (c *Client) AnswerStream(ctx context.Context, p AnswerParams, opts ...RequestOption) (*AnswerStream, error) {
	p.Stream = true
	if c.apiKey == "" {
		return nil, &Error{Code: "missing_api_key", Message: "no API key: use serpkite.WithAPIKey or set SERPKITE_API_KEY"}
	}
	var o callOpts
	for _, f := range opts {
		f(&o)
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("serpkite: encode request: %w", err)
	}
	for attempt := 0; ; attempt++ {
		canRetry := attempt < c.maxRetries
		res, cancel, err := c.openStream(ctx, payload, o.header)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if canRetry {
				if err := sleep(ctx, c.backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
			cancel()
			e := errorFromResponse(res.StatusCode, res.Header, body)
			if canRetry && e.Retryable() {
				if err := sleep(ctx, c.retryDelay(attempt, res.Header)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, e
		}
		if o.info != nil {
			*o.info = responseInfo(res.StatusCode, res.Header)
		}
		rid := res.Header.Get("X-Request-Id")
		mt, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
		if mt == "application/json" {
			// A plain JSON answer (e.g. from a proxy that buffered the stream):
			// replay it as "results" + "done".
			body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
			res.Body.Close()
			cancel()
			var a AnswerResponse
			if err == nil {
				err = json.Unmarshal(body, &a)
			}
			if err != nil {
				return nil, &Error{Status: res.StatusCode, Code: "invalid_response", Message: "decode answer: " + err.Error(), RequestID: rid, Header: res.Header, cause: err}
			}
			done := &AnswerStreamDone{Answer: a.Text(), Citations: a.Citations, Meta: a.Meta}
			empty := io.NopCloser(strings.NewReader(""))
			return &AnswerStream{body: empty, cancel: func() {}, sc: bufio.NewScanner(empty), requestID: rid, queued: []AnswerEvent{
				{Type: AnswerEventResults, Results: &AnswerStreamResults{Request: a.Request, Results: a.Results}},
				{Type: AnswerEventDone, Done: done},
			}}, nil
		}
		if mt != "text/event-stream" {
			res.Body.Close()
			cancel()
			return nil, &Error{Status: res.StatusCode, Code: "invalid_response", Message: "expected text/event-stream, got " + mt, RequestID: rid, Header: res.Header}
		}
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
		return &AnswerStream{body: res.Body, cancel: cancel, sc: sc, requestID: rid}, nil
	}
}

// openStream sends one POST /v1/answer attempt. The client's timeout bounds
// the wait for the response headers only, not the stream that follows; the
// returned cancel ends the request and must be called once the body is done.
func (c *Client) openStream(ctx context.Context, payload []byte, extra http.Header) (*http.Response, context.CancelFunc, error) {
	actx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(actx, http.MethodPost, c.baseURL+"/v1/answer", bytes.NewReader(payload))
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("serpkite: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream, application/json;q=0.9")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "serpkite-go/"+Version)
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	var timer *time.Timer
	if c.timeout > 0 {
		timer = time.AfterFunc(max(c.timeout, answerMinTimeout), cancel)
	}
	res, err := c.httpClient.Do(req)
	timedOut := timer != nil && !timer.Stop()
	if err != nil {
		cancel()
		if timedOut && ctx.Err() == nil {
			return nil, nil, &Error{Code: "timeout", Message: "request timed out", cause: err}
		}
		return nil, nil, transportError(ctx, actx, err)
	}
	return res, cancel, nil
}

// AnswerStream reads the events of [Client.AnswerStream]. It is not safe for
// concurrent use.
type AnswerStream struct {
	body      io.ReadCloser
	cancel    context.CancelFunc
	sc        *bufio.Scanner
	requestID string
	ev        AnswerEvent
	queued    []AnswerEvent // events of a plain JSON answer, replayed before reading
	err       error
	done      bool
}

// Next advances to the next event. It returns false after the "done" event,
// on an "error" event and on read errors; then check [AnswerStream.Err].
func (s *AnswerStream) Next() bool {
	if s.done || s.err != nil {
		return false
	}
	if len(s.queued) > 0 {
		s.ev, s.queued = s.queued[0], s.queued[1:]
		s.done = s.ev.Done != nil
		return true
	}
	var event string
	var data strings.Builder
	hasData := false
	for s.sc.Scan() {
		line := strings.TrimSuffix(s.sc.Text(), "\r")
		switch {
		case line == "":
			if !hasData {
				// An event without data lines is not dispatched (SSE spec).
				event = ""
				continue
			}
			ok := s.dispatch(event, []byte(data.String()))
			event = ""
			data.Reset()
			hasData = false
			if ok {
				return true
			}
			if s.err != nil || s.done {
				return false
			}
		case strings.HasPrefix(line, ":"):
			// comment / keep-alive
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if hasData {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			hasData = true
		}
	}
	if err := s.sc.Err(); err != nil {
		s.err = &Error{Code: "connection_error", Message: "stream read failed: " + err.Error(), RequestID: s.requestID, cause: err}
		return false
	}
	if hasData && s.dispatch(event, []byte(data.String())) {
		return true
	}
	if !s.done && s.err == nil {
		s.err = &Error{Code: "invalid_response", Message: "the stream ended before the done event", RequestID: s.requestID}
	}
	return false
}

// dispatch decodes one event. It returns true when the event is delivered to
// the caller.
func (s *AnswerStream) dispatch(event string, data []byte) bool {
	decode := func(v any) bool {
		if err := json.Unmarshal(data, v); err != nil {
			s.err = &Error{Code: "invalid_response", Message: "decode " + event + " event: " + err.Error(), RequestID: s.requestID}
			return false
		}
		return true
	}
	switch event {
	case AnswerEventResults:
		var v AnswerStreamResults
		if !decode(&v) {
			return false
		}
		s.ev = AnswerEvent{Type: event, Results: &v}
	case AnswerEventDelta:
		var v AnswerStreamDelta
		if !decode(&v) {
			return false
		}
		s.ev = AnswerEvent{Type: event, Delta: &v}
	case AnswerEventDone:
		var v AnswerStreamDone
		if !decode(&v) {
			return false
		}
		s.ev = AnswerEvent{Type: event, Done: &v}
		s.done = true
	case "error":
		var env errorEnvelope
		_ = json.Unmarshal(data, &env)
		e := &Error{Status: http.StatusServiceUnavailable, Code: env.Error.Code, Message: env.Error.Message, RequestID: env.Error.RequestID}
		if e.Code == "" {
			e.Code, e.Message = "ai_unavailable", strings.TrimSpace(string(data))
		}
		if e.RequestID == "" {
			e.RequestID = s.requestID
		}
		s.err = e
		return false
	default:
		return false // unknown events are skipped
	}
	return true
}

// Event returns the current event (valid after Next returned true).
func (s *AnswerStream) Event() AnswerEvent { return s.ev }

// Err returns the error that stopped the stream: an *Error with the code of
// an "error" event (e.g. "ai_unavailable"; Status is 503 though the stream
// itself was a 200), or a read failure. Nil after a "done" event.
func (s *AnswerStream) Err() error { return s.err }

// Close releases the connection. Closing before "done" stops generation and
// the model part is not charged.
func (s *AnswerStream) Close() error {
	err := s.body.Close()
	s.cancel()
	return err
}

// Collect reads the rest of the stream, closes it and returns the "done" event.
func (s *AnswerStream) Collect() (*AnswerStreamDone, error) {
	defer s.Close()
	for s.Next() {
		if s.ev.Done != nil {
			return s.ev.Done, nil
		}
	}
	if s.err == nil {
		return nil, &Error{Code: "invalid_response", Message: "the stream ended before the done event", RequestID: s.requestID}
	}
	return nil, s.err
}

// ── Research ─────────────────────────────────────────────────────────────

// Research starts an async research task: planned Google searches, page
// reading and a cited Markdown report ([ResearchResult].Report; with
// OutputSchema also Output). The flat price of its Effort is reserved up
// front and charged only when the report is written. Poll it with
// [Client.GetResearch] or [Client.WaitResearch], or receive the signed
// research.completed webhook. Like [Client.Crawl] it is only retried on 429,
// so a lost response never starts (and reserves) a second task.
func (c *Client) Research(ctx context.Context, p ResearchParams, opts ...RequestOption) (*TaskCreated, error) {
	var out TaskCreated
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: "/v1/research", body: p, opts: opts, out: &out, rateLimitOnly: true}); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetResearch polls a research task (Progress while it runs; kept for 24 hours).
func (c *Client) GetResearch(ctx context.Context, id string, opts ...RequestOption) (*ResearchTask, error) {
	var out ResearchTask
	if _, err := c.decode(ctx, call{method: http.MethodGet, path: "/v1/research/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelResearch cancels a research task: a queued one is refunded at once, a
// running one stops at its next round and is charged for the searches already
// run. A finished task is a 409.
func (c *Client) CancelResearch(ctx context.Context, id string, opts ...RequestOption) (*TaskCancelResponse, error) {
	var out TaskCancelResponse
	if _, err := c.decode(ctx, call{method: http.MethodDelete, path: "/v1/research/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitResearch polls a research task until it completed, failed or was
// canceled (default timeout 15 minutes; polling every 2 s backing off to
// 15 s). A failed task is returned without an error: check Status and Error.
// When the timeout passes it returns the last poll and an *Error with code
// "timeout".
func (c *Client) WaitResearch(ctx context.Context, id string, opts ...WaitOptions) (*ResearchTask, error) {
	return waitTask(ctx, "research task", id, 15*time.Minute, opts, c.GetResearch, func(t *ResearchTask) string { return t.Status })
}
