package serpkite

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ── Crawl ────────────────────────────────────────────────────────────────

// Crawl starts an async crawl of one site: from the start page and (unless
// Sitemap is "skip") the site's sitemaps, breadth first or best first with
// Query, honouring each host's robots.txt and Crawl-delay. A crawl runs for
// at most 30 minutes and 16 MB of pages; [CrawlStats].Stopped says why it
// ended. Limit credits are reserved up front; each page read costs 1 (0.5
// cached) and the rest is refunded. Like [BatchesService.Create] it is only
// retried on 429, so a lost response never starts (and reserves) a second
// crawl.
func (c *Client) Crawl(ctx context.Context, p CrawlParams, opts ...RequestOption) (*TaskCreated, error) {
	var out TaskCreated
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: "/v1/crawl", body: p, opts: opts, out: &out, rateLimitOnly: true}); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCrawl polls a crawl (kept for 24 hours).
func (c *Client) GetCrawl(ctx context.Context, id string, opts ...RequestOption) (*CrawlTask, error) {
	var out CrawlTask
	if _, err := c.decode(ctx, call{method: http.MethodGet, path: "/v1/crawl/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelCrawl cancels a crawl: a queued one is refunded at once, a running
// one stops at its next checkpoint and is charged for the pages read. A
// finished crawl is a 409.
func (c *Client) CancelCrawl(ctx context.Context, id string, opts ...RequestOption) (*TaskCancelResponse, error) {
	var out TaskCancelResponse
	if _, err := c.decode(ctx, call{method: http.MethodDelete, path: "/v1/crawl/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitForCrawl polls a crawl until it completed, failed or was canceled
// (default timeout 35 minutes: a crawl runs for up to 30; polling every 2 s
// backing off to 15 s). A
// failed crawl is returned without an error: check Status and Error. When
// the timeout passes it returns the last poll and an *Error with code
// "timeout".
func (c *Client) WaitForCrawl(ctx context.Context, id string, opts ...WaitOptions) (*CrawlTask, error) {
	return waitTask(ctx, "crawl", id, 35*time.Minute, opts, c.GetCrawl, func(t *CrawlTask) string { return t.Status })
}

func waitTask[T any](ctx context.Context, kind, id string, defTimeout time.Duration, opts []WaitOptions,
	get func(context.Context, string, ...RequestOption) (*T, error), status func(*T) string,
) (*T, error) {
	var o WaitOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Timeout <= 0 {
		o.Timeout = defTimeout
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	if o.MaxPollInterval <= 0 {
		o.MaxPollInterval = 15 * time.Second
	}
	if id == "" {
		return nil, &Error{Code: "invalid_request", Message: "waiting for a " + kind + " needs a task id"}
	}
	deadline := time.Now().Add(o.Timeout)
	interval := o.PollInterval
	for {
		t, err := get(ctx, id)
		if err != nil {
			return nil, err
		}
		if taskFinished(status(t)) {
			return t, nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return t, &Error{Code: "timeout", Message: fmt.Sprintf("%s %s still %s after %s", kind, id, status(t), o.Timeout)}
		}
		if err := sleep(ctx, min(interval, left)); err != nil {
			return nil, err
		}
		interval = min(time.Duration(float64(interval)*1.5), o.MaxPollInterval)
	}
}

// ── Monitors ─────────────────────────────────────────────────────────────

// MonitorsService manages monitors: saved searches (or watched pages) that
// run on a schedule and report what is new, to a signed webhook
// (monitor.results) when the monitor has one and always in its run history
// ([MonitorsService.Runs]). Each run costs the price of its request (1
// credit; empty search runs are free). A monitor pauses itself after 10
// failed runs in a row; Update with Active true resumes it.
type MonitorsService struct{ c *Client }

// Create saves a monitor; its first run is due right away (unless Active is
// false). Only retried on 429.
func (s *MonitorsService) Create(ctx context.Context, p MonitorCreateParams, opts ...RequestOption) (*Monitor, error) {
	var out Monitor
	if _, err := s.c.decode(ctx, call{method: http.MethodPost, path: "/v1/monitors", body: p, opts: opts, out: &out, rateLimitOnly: true}); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the account's monitors.
func (s *MonitorsService) List(ctx context.Context, opts ...RequestOption) ([]Monitor, error) {
	var out MonitorList
	if _, err := s.c.decode(ctx, call{method: http.MethodGet, path: "/v1/monitors", opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// Get returns one monitor.
func (s *MonitorsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Monitor, error) {
	var out Monitor
	if _, err := s.c.decode(ctx, call{method: http.MethodGet, path: "/v1/monitors/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes a monitor's name, interval, webhook (String("") removes it),
// active flag, metadata and saved request. A changed request (or endpoint)
// starts a new baseline: the next run reports every result as new.
func (s *MonitorsService) Update(ctx context.Context, id string, p MonitorUpdateParams, opts ...RequestOption) (*Monitor, error) {
	var out Monitor
	if _, err := s.c.decode(ctx, call{method: http.MethodPatch, path: "/v1/monitors/" + url.PathEscape(id), body: p, opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a monitor.
func (s *MonitorsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	_, _, err := s.c.do(ctx, call{method: http.MethodDelete, path: "/v1/monitors/" + url.PathEscape(id), opts: opts})
	return err
}

// Run makes a monitor due now (it runs at the next scheduler poll, within
// about 30 seconds). Only retried on 429.
func (s *MonitorsService) Run(ctx context.Context, id string, opts ...RequestOption) (*Monitor, error) {
	var out Monitor
	if _, err := s.c.decode(ctx, call{method: http.MethodPost, path: "/v1/monitors/" + url.PathEscape(id) + "/run", opts: opts, out: &out, rateLimitOnly: true}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Runs returns a monitor's run history, newest first (runs are kept 30 days,
// their results 24 hours). Pass the returned NextBefore as Before for the
// next page; p may be nil.
func (s *MonitorsService) Runs(ctx context.Context, id string, p *MonitorRunsParams, opts ...RequestOption) (*MonitorRunList, error) {
	path := "/v1/monitors/" + url.PathEscape(id) + "/runs"
	if p != nil {
		q := url.Values{}
		if p.Limit > 0 {
			q.Set("limit", strconv.Itoa(p.Limit))
		}
		if p.Before != "" {
			q.Set("before", p.Before)
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
	}
	var out MonitorRunList
	if _, err := s.c.decode(ctx, call{method: http.MethodGet, path: path, opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}
