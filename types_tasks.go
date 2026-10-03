package serpkite

import (
	"encoding/json"
	"time"
)

// Int returns a pointer to v, for *int fields where 0 is a valid value
// ([CrawlParams].MaxDepth).
func Int(v int) *int { return &v }

// String returns a pointer to v, for the *string fields of [MonitorUpdateParams].
func String(v string) *string { return &v }

// ── Async tasks: crawl ───────────────────────────────────────────────────

// Task statuses.
const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskCompleted = "completed"
	TaskFailed    = "failed"
	TaskCanceled  = "canceled"
)

// taskFinished reports whether a task status is final.
func taskFinished(status string) bool {
	return status == TaskCompleted || status == TaskFailed || status == TaskCanceled
}

// CrawlParams are the parameters of [Client.Crawl].
type CrawlParams struct {
	// URL is the start page (required). It is always read: path filters and
	// robots.txt apply to the pages found from it.
	URL string `json:"url"`
	// Limit is the maximum pages read (1-1000, default 25); that many credits are reserved.
	Limit int `json:"limit,omitempty"`
	// MaxDepth is the link hops from URL (0-10, default 2; sitemap pages count
	// as 1 hop, 0 reads only URL). Use [Int] (0 is valid).
	MaxDepth *int `json:"max_depth,omitempty"`
	// IncludePaths and ExcludePaths are regular expressions matched against the URL path.
	IncludePaths      []string `json:"include_paths,omitempty"`
	ExcludePaths      []string `json:"exclude_paths,omitempty"`
	IncludeSubdomains bool     `json:"include_subdomains,omitempty"`
	// Sitemap is "include" (default: the site's sitemap URLs also seed the
	// crawl, after the start page's links), "only" (the start page and sitemap
	// URLs, no links followed) or "skip" (links only).
	Sitemap string `json:"sitemap,omitempty"`
	// Query makes the crawl best first: pages whose URL and link text match
	// these words are read first.
	Query string `json:"query,omitempty"`
	// IgnoreQueryParameters treats URLs that differ only in their query string as one page.
	IgnoreQueryParameters bool `json:"ignore_query_parameters,omitempty"`
	// Format is "markdown" (default) or "text".
	Format       string `json:"format,omitempty"`
	IncludeLinks bool   `json:"include_links,omitempty"`
	MaxTokens    int    `json:"max_tokens,omitempty"`
	MaxAge       int    `json:"max_age,omitempty"`
	// WebhookURL receives the signed crawl.completed event (default: the account webhook).
	WebhookURL string `json:"webhook_url,omitempty"`
}

// TaskCreated is the 202 response of [Client.Crawl].
type TaskCreated struct {
	ID string `json:"id"`
	// Kind is "crawl".
	Kind            string    `json:"kind"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	PollURL         string    `json:"poll_url"`
	CreditsReserved float64   `json:"credits_reserved"`
	WebhookURL      *string   `json:"webhook_url,omitempty"`
}

// TaskCancelResponse is returned by [Client.CancelCrawl].
type TaskCancelResponse struct {
	ID string `json:"id"`
	// Status is "canceled" (it was queued: refunded at once) or "canceling"
	// (running; it stops at its next checkpoint).
	Status string `json:"status"`
}

// CrawlProgress is the progress of a running crawl.
type CrawlProgress struct {
	PagesDone       int `json:"pages_done,omitempty"`
	PagesFailed     int `json:"pages_failed,omitempty"`
	PagesQueued     int `json:"pages_queued,omitempty"`
	PagesDiscovered int `json:"pages_discovered,omitempty"`
	Limit           int `json:"limit,omitempty"`
}

// CrawlPage is one page a crawl read.
type CrawlPage struct {
	URL string `json:"url"`
	// Depth is the link hops from the start page.
	Depth       int           `json:"depth"`
	Title       string        `json:"title,omitempty"`
	Markdown    string        `json:"markdown,omitempty"`
	Text        string        `json:"text,omitempty"`
	PublishedAt string        `json:"published_at,omitempty"`
	Metadata    *PageMetadata `json:"metadata,omitempty"`
	Links       []PageLink    `json:"links,omitempty"`
}

// CrawlStats summarises a crawl.
type CrawlStats struct {
	Pages   int `json:"pages,omitempty"`
	Failed  int `json:"failed,omitempty"`
	Seconds int `json:"seconds,omitempty"`
	// Discovered is the unique URLs found that passed the filters.
	Discovered int `json:"discovered,omitempty"`
	// Queued is the URLs found but not read when the crawl ended.
	Queued int `json:"queued,omitempty"`
	// Duplicates is the pages that redirected to a page already read (not charged).
	Duplicates int `json:"duplicates,omitempty"`
	// SitemapURLs is the URLs taken from sitemaps.
	SitemapURLs int `json:"sitemap_urls,omitempty"`
	// Robots is the start host's robots.txt: found, missing, unreachable or none.
	Robots string `json:"robots,omitempty"`
	// RobotsDisallowed is the number of Disallow rules in force on the start host.
	RobotsDisallowed int `json:"robots_disallowed,omitempty"`
	// RobotsBlocked is the URLs skipped because robots.txt disallows them.
	RobotsBlocked int `json:"robots_blocked,omitempty"`
	// CrawlDelayMs is the start host's Crawl-delay, honoured.
	CrawlDelayMs int `json:"crawl_delay_ms,omitempty"`
	// Stopped is why the crawl ended: done, limit, time_limit, size_limit,
	// too_many_failures or canceled.
	Stopped string `json:"stopped,omitempty"`
}

// CrawlResult is the result of a crawl.
type CrawlResult struct {
	URL    string           `json:"url"`
	Pages  []CrawlPage      `json:"pages"`
	Failed []ExtractFailure `json:"failed"`
	Stats  CrawlStats       `json:"stats"`
}

// CrawlTask is returned by [Client.GetCrawl].
type CrawlTask struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreditsReserved float64    `json:"credits_reserved"`
	CreditsUsed     float64    `json:"credits_used"`
	// WebhookStatus is pending, delivered, failed or nil.
	WebhookStatus *string        `json:"webhook_status,omitempty"`
	Progress      *CrawlProgress `json:"progress,omitempty"`
	// Result is set once the crawl ends (also on cancel, with the pages read so far).
	Result *CrawlResult `json:"result,omitempty"`
	Error  *TaskError   `json:"error,omitempty"`
}

// Finished reports whether the crawl completed, failed or was canceled.
func (t *CrawlTask) Finished() bool { return taskFinished(t.Status) }

// TaskError says why a task failed or was canceled: code is no_pages,
// canceled, timeout, internal…
type TaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ── Monitors ─────────────────────────────────────────────────────────────

// Monitor intervals.
const (
	IntervalHourly = "hourly"
	IntervalDaily  = "daily"
	IntervalWeekly = "weekly"
)

// MonitorCreateParams are the parameters of [MonitorsService.Create]. Search
// and news monitors need Q; webpage monitors need URL and take only Country
// besides (search options on a webpage monitor are a 400).
type MonitorCreateParams struct {
	// Q is the saved query (search and news monitors).
	Q string `json:"q,omitempty"`
	// URL is the page to watch for content changes (Endpoint "webpage").
	URL string `json:"url,omitempty"`
	// Endpoint is "search" (default), "news" or "webpage" (1 credit per check;
	// reports the page when its content changes, as a [MonitorPageChange]).
	Endpoint string `json:"endpoint,omitempty"`
	// Metadata is your own JSON object (at most 2 KB), echoed on the monitor
	// and in its webhooks.
	Metadata map[string]any `json:"metadata,omitempty"`
	Name     string         `json:"name,omitempty"`
	// Interval is hourly, daily (default) or weekly.
	Interval string `json:"interval,omitempty"`
	// IntervalSeconds (3600-2592000) overrides Interval.
	IntervalSeconds int `json:"interval_seconds,omitempty"`
	// WebhookURL receives signed monitor.results events. Without one, read
	// new results from the run history ([MonitorsService.Runs]).
	WebhookURL string `json:"webhook_url,omitempty"`
	// Active false creates the monitor paused (default true). Use [Bool].
	Active         *bool    `json:"active,omitempty"`
	Country        string   `json:"country,omitempty"`
	Language       string   `json:"language,omitempty"`
	Location       string   `json:"location,omitempty"`
	Time           string   `json:"time,omitempty"`
	Num            int      `json:"num,omitempty"`
	Device         string   `json:"device,omitempty"`
	Safe           string   `json:"safe,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	Engine         Engine   `json:"engine,omitempty"`
}

// ClearMetadata, as [MonitorUpdateParams].Metadata, removes a monitor's metadata.
var ClearMetadata = json.RawMessage("null")

// MonitorUpdateParams are the parameters of [MonitorsService.Update]. Nil and
// zero fields are left unchanged. The search fields (Endpoint, Q, URL and the
// search options) are merged into the saved search; a changed search makes
// the next run report every result as new. Switching Endpoint between a
// search ("search", "news") and "webpage" drops the other kind's fields.
type MonitorUpdateParams struct {
	Name            *string `json:"name,omitempty"`
	Interval        string  `json:"interval,omitempty"`
	IntervalSeconds int     `json:"interval_seconds,omitempty"`
	// WebhookURL sets the webhook; String("") removes it (new results are then
	// only kept in the run history).
	WebhookURL *string `json:"webhook_url,omitempty"`
	// Active true resumes a paused monitor (due now, failure streak cleared).
	Active *bool `json:"active,omitempty"`
	// Metadata replaces the monitor's metadata: a JSON object (for example
	// from json.Marshal), or [ClearMetadata] to remove it. Nil leaves it unchanged.
	Metadata json.RawMessage `json:"metadata,omitempty"`
	// Endpoint is "search", "news" or "webpage".
	Endpoint string `json:"endpoint,omitempty"`
	Q        string `json:"q,omitempty"`
	// URL is the watched page (webpage monitors only).
	URL            string   `json:"url,omitempty"`
	Country        string   `json:"country,omitempty"`
	Language       string   `json:"language,omitempty"`
	Location       string   `json:"location,omitempty"`
	Time           string   `json:"time,omitempty"`
	Num            int      `json:"num,omitempty"`
	Device         string   `json:"device,omitempty"`
	Safe           string   `json:"safe,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	Engine         Engine   `json:"engine,omitempty"`
}

// MonitorSearch is the saved request a monitor reruns: a search (Q and
// options) or, for webpage monitors, URL (and Country).
type MonitorSearch struct {
	Q              string   `json:"q,omitempty"`
	URL            string   `json:"url,omitempty"`
	Country        string   `json:"country,omitempty"`
	Language       string   `json:"language,omitempty"`
	Location       string   `json:"location,omitempty"`
	Time           string   `json:"time,omitempty"`
	Num            int      `json:"num,omitempty"`
	Device         string   `json:"device,omitempty"`
	Safe           string   `json:"safe,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	Engine         []string `json:"engine,omitempty"`
}

// Monitor is a scheduled search.
type Monitor struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Endpoint string        `json:"endpoint"`
	Request  MonitorSearch `json:"request"`
	// Metadata is your own JSON object; nil when unset.
	Metadata        map[string]any `json:"metadata,omitempty"`
	IntervalSeconds int            `json:"interval_seconds"`
	// WebhookURL is nil when new results are only kept in the run history.
	WebhookURL *string    `json:"webhook_url"`
	Active     bool       `json:"active"`
	NextRunAt  time.Time  `json:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	// LastStatus is ok, error, webhook_failed, paused or nil.
	LastStatus *string `json:"last_status,omitempty"`
	LastError  *string `json:"last_error,omitempty"`
	// LastNewResults is the number of new results the last run found.
	LastNewResults int `json:"last_new_results,omitempty"`
	Runs           int `json:"runs"`
	// ConsecutiveFailures is the failed runs in a row; the monitor pauses itself at 10.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// CreditsUsed is the total over all runs.
	CreditsUsed float64   `json:"credits_used"`
	CreatedAt   time.Time `json:"created_at"`
}

// MonitorList is returned by [MonitorsService.List].
type MonitorList struct {
	Results []Monitor `json:"results"`
}

// Webpage monitor change kinds ([MonitorPageChange].Change).
const (
	PageChangeNew     = "new"
	PageChangeChanged = "changed"
)

// MonitorPageChange is what a webpage monitor reports (in [MonitorRun].Results
// and [MonitorResultsEvent].NewResults) when the page is new to it or its
// content changed.
type MonitorPageChange struct {
	// URL is the final URL after redirects.
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
	// Change is PageChangeNew or PageChangeChanged.
	Change string `json:"change"`
	// ContentHash fingerprints the content (whitespace-insensitive).
	ContentHash string `json:"content_hash"`
	// Markdown is the page (up to about 8,000 tokens).
	Markdown    string `json:"markdown"`
	PublishedAt string `json:"published_at,omitempty"`
}

// MonitorRunsParams are the parameters of [MonitorsService.Runs].
type MonitorRunsParams struct {
	// Limit is 1-100 (default 20).
	Limit int
	// Before is NextBefore of the previous page.
	Before string
}

// MonitorRun is one run of a monitor.
type MonitorRun struct {
	ID string `json:"id"`
	// Status is ok, error, webhook_failed or paused.
	Status     string  `json:"status"`
	Error      *string `json:"error,omitempty"`
	NewResults int     `json:"new_results"`
	// Results are the new results ([OrganicResult]-shaped for search monitors,
	// [NewsResult] for news, [MonitorPageChange] for webpage); nil when there
	// were none or after 24 hours.
	Results     []json.RawMessage `json:"results,omitempty"`
	CreditsUsed float64           `json:"credits_used"`
	// WebhookStatus is delivered, failed, none (the monitor has no webhook)
	// or nil (nothing to send).
	WebhookStatus *string   `json:"webhook_status,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// MonitorRunList is returned by [MonitorsService.Runs], newest first.
type MonitorRunList struct {
	Results []MonitorRun `json:"results"`
	// NextBefore is passed as [MonitorRunsParams].Before for the next page;
	// nil on the last page.
	NextBefore *string `json:"next_before"`
}

// ── Webhook events ───────────────────────────────────────────────────────

// Webhook event types (the X-SerpKite-Event header and the "event" field).
const (
	EventBatchCompleted = "batch.completed"
	EventCrawlCompleted = "crawl.completed"

	EventMonitorResults = "monitor.results"
)

// TaskCompletedEvent is the body of crawl.completed.
type TaskCompletedEvent struct {
	Event       string     `json:"event"`
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreditsUsed float64    `json:"credits_used"`
	// PollURL is where the task (and its full result) can be fetched for 24 hours.
	PollURL string `json:"poll_url,omitempty"`
	// Result is a [CrawlResult]; decode it with DecodeResult.
	// It is null when the task failed without one, or when it was larger than
	// 4 MB (ResultOmitted: fetch PollURL, e.g. with [Client.GetCrawl]).
	Result json.RawMessage `json:"result,omitempty"`
	// ResultOmitted is set when the result was too large for the webhook.
	ResultOmitted bool       `json:"result_omitted,omitempty"`
	Error         *TaskError `json:"error,omitempty"`
}

// DecodeResult unmarshals Result into v (a *CrawlResult).
// It returns false when there is no result.
func (e *TaskCompletedEvent) DecodeResult(v any) (bool, error) {
	if len(e.Result) == 0 || string(e.Result) == "null" {
		return false, nil
	}
	return true, json.Unmarshal(e.Result, v)
}

// MonitorResultsEvent is the body of monitor.results.
type MonitorResultsEvent struct {
	Event     string `json:"event"`
	MonitorID string `json:"monitor_id"`
	// RunID is also the X-SerpKite-Delivery header; use it to deduplicate retries.
	RunID    string `json:"run_id"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	// Q is the saved query (empty for webpage monitors).
	Q string `json:"q"`
	// URL is the watched page (webpage monitors only).
	URL string `json:"url,omitempty"`
	// Metadata is the monitor's metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
	// FirstRun is the first run since the monitor was created or its search changed.
	FirstRun bool      `json:"first_run"`
	RunAt    time.Time `json:"run_at"`
	// NewResults are results not seen in earlier runs: [OrganicResult]-shaped
	// for search monitors, [NewsResult] for news, [MonitorPageChange] for webpage.
	NewResults  []json.RawMessage `json:"new_results"`
	CreditsUsed float64           `json:"credits_used"`
}
