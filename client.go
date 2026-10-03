// Package serpkite is the official Go client for SerpKite (serpkite.com), the
// Google search API built for AI agents: clean JSON or Markdown results.
//
//	c := serpkite.NewClient() // reads SERPKITE_API_KEY
//	res, err := c.Search(ctx, serpkite.SearchParams{Q: "best espresso machine", Country: "us"})
//	fmt.Println(res.Results[0].Title, res.Meta.CreditsUsed)
package serpkite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, sent in the User-Agent.
const Version = "0.2.0"

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://api.serpkite.com"

const (
	defaultTimeout    = 60 * time.Second
	defaultMaxRetries = 2
	retryBase         = 500 * time.Millisecond
	retryCap          = 8 * time.Second
	retryAfterCap     = 60 * time.Second
)

// Client calls the SerpKite API. It is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	timeout    time.Duration
	retryBase  time.Duration

	// Batches queues and polls half-price batch jobs.
	Batches *BatchesService
}

// Option configures a [Client].
type Option func(*Client)

// WithAPIKey sets the API key (skt_live_…). Default: the SERPKITE_API_KEY environment variable.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL overrides the API origin. Default: SERPKITE_BASE_URL or https://api.serpkite.com.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient sets the underlying HTTP client. Default: a dedicated http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets how many times a request is retried after the first
// attempt on 429, 5xx and network errors. Default 2; 0 disables retries.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = max(n, 0) } }

// WithTimeout sets the per-attempt timeout. Default 60 s; 0 disables it (use the context).
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithRetryBackoff sets the base delay of the exponential backoff. Default 500 ms.
func WithRetryBackoff(d time.Duration) Option { return func(c *Client) { c.retryBase = d } }

// NewClient returns a client. Without [WithAPIKey] it reads SERPKITE_API_KEY;
// a missing key surfaces as an *Error with code "missing_api_key" on the first call.
func NewClient(opts ...Option) *Client {
	c := &Client{
		apiKey:     os.Getenv("SERPKITE_API_KEY"),
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{},
		maxRetries: defaultMaxRetries,
		timeout:    defaultTimeout,
		retryBase:  retryBase,
	}
	if u := os.Getenv("SERPKITE_BASE_URL"); u != "" {
		c.baseURL = strings.TrimRight(u, "/")
	}
	for _, o := range opts {
		o(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	c.Batches = &BatchesService{c: c}
	return c
}

// ── Per-request options ──────────────────────────────────────────────────

// ResponseInfo holds the billing and tracing headers of a response.
type ResponseInfo struct {
	StatusCode int
	Header     http.Header
	RequestID  string
	// CreditsUsed and CreditsRemaining come from X-Credits-Used / X-Credits-Remaining.
	CreditsUsed      float64
	CreditsRemaining *float64
	CostUSD          float64
	// Cache is "HIT" (served from cache via MaxAge) or "MISS".
	Cache          string
	LatencyMs      int
	TokensEstimate int
}

// RequestOption customises one call.
type RequestOption func(*callOpts)

type callOpts struct {
	info   *ResponseInfo
	header http.Header
	// idempotent: the call carries an Idempotency-Key, so 5xx and network
	// errors are safe to retry.
	idempotent bool
}

// CaptureResponse stores the response headers (credits used/remaining, cache,
// request id) of a successful call in info.
func CaptureResponse(info *ResponseInfo) RequestOption {
	return func(o *callOpts) { o.info = info }
}

// WithHeader adds a header to one call, e.g. X-Request-Id for tracing.
func WithHeader(key, value string) RequestOption {
	return func(o *callOpts) {
		if o.header == nil {
			o.header = http.Header{}
		}
		o.header.Add(key, value)
	}
}

// WithIdempotencyKey sends Idempotency-Key on [BatchesService.Create]
// (1-255 printable ASCII characters, e.g. a UUID). For 24 hours a retry with
// the same key and body returns the first response instead of queueing and
// billing the jobs again, so Create then also retries 5xx and network errors.
func WithIdempotencyKey(key string) RequestOption {
	return func(o *callOpts) {
		if o.header == nil {
			o.header = http.Header{}
		}
		o.header.Set("Idempotency-Key", key)
		o.idempotent = true
	}
}

// ── Verticals ────────────────────────────────────────────────────────────

// Search runs a Google web search: organic results, answer box, knowledge
// graph, people also ask, top stories, local pack.
func (c *Client) Search(ctx context.Context, p SearchParams, opts ...RequestOption) (*SearchResponse, error) {
	return post[SearchResponse](ctx, c, "/v1/search", p, opts)
}

// SearchMarkdown runs a web search and returns the result as Markdown.
func (c *Client) SearchMarkdown(ctx context.Context, p SearchParams, opts ...RequestOption) (string, error) {
	p.Format = FormatMarkdown
	return c.markdown(ctx, "/v1/search", p, opts)
}

// Markdown calls any vertical with format=markdown and returns the Markdown.
// params is the endpoint's params struct (SearchParams, ReviewsParams,
// WebpageParams) or a map[string]any.
func (c *Client) Markdown(ctx context.Context, endpoint Endpoint, params any, opts ...RequestOption) (string, error) {
	body, err := withField(params, "format", FormatMarkdown)
	if err != nil {
		return "", err
	}
	return c.markdown(ctx, "/v1/"+string(endpoint), body, opts)
}

// Images searches Google Images.
func (c *Client) Images(ctx context.Context, p SearchParams, opts ...RequestOption) (*ImagesResponse, error) {
	return post[ImagesResponse](ctx, c, "/v1/images", p, opts)
}

// Videos searches Google Videos.
func (c *Client) Videos(ctx context.Context, p SearchParams, opts ...RequestOption) (*VideosResponse, error) {
	return post[VideosResponse](ctx, c, "/v1/videos", p, opts)
}

// News searches Google News.
func (c *Client) News(ctx context.Context, p SearchParams, opts ...RequestOption) (*NewsResponse, error) {
	return post[NewsResponse](ctx, c, "/v1/news", p, opts)
}

// Maps searches Google Maps (places with coordinates). Set LL to "@lat,lng,14z" for a viewport.
func (c *Client) Maps(ctx context.Context, p SearchParams, opts ...RequestOption) (*PlacesResponse, error) {
	return post[PlacesResponse](ctx, c, "/v1/maps", p, opts)
}

// Places returns Google local results.
func (c *Client) Places(ctx context.Context, p SearchParams, opts ...RequestOption) (*PlacesResponse, error) {
	return post[PlacesResponse](ctx, c, "/v1/places", p, opts)
}

// Reviews returns reviews of a place (by PlaceID, CID or FID). Page with
// PageToken = NextPageToken.
func (c *Client) Reviews(ctx context.Context, p ReviewsParams, opts ...RequestOption) (*ReviewsResponse, error) {
	return post[ReviewsResponse](ctx, c, "/v1/reviews", p, opts)
}

// Shopping searches Google Shopping.
func (c *Client) Shopping(ctx context.Context, p SearchParams, opts ...RequestOption) (*ShoppingResponse, error) {
	return post[ShoppingResponse](ctx, c, "/v1/shopping", p, opts)
}

// Scholar searches Google Scholar.
func (c *Client) Scholar(ctx context.Context, p SearchParams, opts ...RequestOption) (*ScholarResponse, error) {
	return post[ScholarResponse](ctx, c, "/v1/scholar", p, opts)
}

// Patents searches Google Patents.
func (c *Client) Patents(ctx context.Context, p SearchParams, opts ...RequestOption) (*PatentsResponse, error) {
	return post[PatentsResponse](ctx, c, "/v1/patents", p, opts)
}

// Autocomplete returns Google autocomplete suggestions.
func (c *Client) Autocomplete(ctx context.Context, p SearchParams, opts ...RequestOption) (*AutocompleteResponse, error) {
	return post[AutocompleteResponse](ctx, c, "/v1/autocomplete", p, opts)
}

// Webpage fetches a public URL and returns clean Markdown, text and metadata.
// With Format "markdown" only Markdown (and URL) is filled in.
func (c *Client) Webpage(ctx context.Context, p WebpageParams, opts ...RequestOption) (*WebpageResponse, error) {
	var out WebpageResponse
	text, isJSON, err := c.do(ctx, call{method: http.MethodPost, path: "/v1/webpage", body: p, opts: opts, out: &out})
	if err != nil {
		return nil, err
	}
	if !isJSON {
		return &WebpageResponse{URL: p.URL, Markdown: text}, nil
	}
	return &out, nil
}

// Rank finds the position of a domain for a keyword in the top Num results (default 100).
func (c *Client) Rank(ctx context.Context, p RankParams, opts ...RequestOption) (*RankResponse, error) {
	return post[RankResponse](ctx, c, "/v1/rank", p, opts)
}

// Account returns the balance, limits and this month's usage of the key's account.
func (c *Client) Account(ctx context.Context, opts ...RequestOption) (*Account, error) {
	var out Account
	if _, err := c.decode(ctx, call{method: http.MethodGet, path: "/v1/account", opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Do sends a raw request (for endpoints without a typed method). body is
// JSON-encoded when non-nil; the JSON response is decoded into out when out is
// non-nil. It returns the response headers.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...RequestOption) (*ResponseInfo, error) {
	var info ResponseInfo
	opts = append(opts, CaptureResponse(&info))
	_, _, err := c.do(ctx, call{method: method, path: path, body: body, opts: opts, out: out})
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// ── Batches ──────────────────────────────────────────────────────────────

// BatchesService queues requests at half price and polls the jobs.
type BatchesService struct{ c *Client }

// Create queues 1-100 requests for one endpoint. The response has one entry
// per request, in order: a queued job or the error that rejected it. Without
// [WithIdempotencyKey], Create is only retried on 429, so a lost response
// never queues (and bills) the same requests twice.
func (s *BatchesService) Create(ctx context.Context, p BatchCreateParams, opts ...RequestOption) (*BatchCreateResponse, error) {
	var out BatchCreateResponse
	if _, err := s.c.decode(ctx, call{method: http.MethodPost, path: "/v1/batches", body: p, opts: opts, out: &out, rateLimitOnly: true}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get polls one job.
func (s *BatchesService) Get(ctx context.Context, id string, opts ...RequestOption) (*Batch, error) {
	var out Batch
	if _, err := s.c.decode(ctx, call{method: http.MethodGet, path: "/v1/batches/" + url.PathEscape(id), opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitOptions tunes [BatchesService.Wait].
type WaitOptions struct {
	// Timeout bounds the whole wait. Default 10 minutes; the context deadline also applies.
	Timeout time.Duration
	// PollInterval is the first delay between polls (default 1 s); it grows 1.5× per poll.
	PollInterval time.Duration
	// MaxPollInterval caps the delay between polls. Default 10 s.
	MaxPollInterval time.Duration
}

// Wait polls a job until it is done or failed and returns it. A failed job is
// returned without an error: check Status and Error. When the timeout passes
// it returns an *Error with code "timeout".
func (s *BatchesService) Wait(ctx context.Context, id string, opts ...WaitOptions) (*Batch, error) {
	var o WaitOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Minute
	}
	if o.PollInterval <= 0 {
		o.PollInterval = time.Second
	}
	if o.MaxPollInterval <= 0 {
		o.MaxPollInterval = 10 * time.Second
	}
	if id == "" {
		return nil, &Error{Code: "invalid_request", Message: "batches.Wait needs a batch id"}
	}
	deadline := time.Now().Add(o.Timeout)
	interval := o.PollInterval
	for {
		b, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if b.Finished() {
			return b, nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return b, &Error{Code: "timeout", Message: fmt.Sprintf("batch %s still %s after %s", id, b.Status, o.Timeout)}
		}
		if err := sleep(ctx, min(interval, left)); err != nil {
			return nil, err
		}
		interval = min(time.Duration(float64(interval)*1.5), o.MaxPollInterval)
	}
}

// ── Plumbing ─────────────────────────────────────────────────────────────

type call struct {
	method, path  string
	body          any
	out           any
	opts          []RequestOption
	rateLimitOnly bool
}

func post[T any](ctx context.Context, c *Client, path string, body any, opts []RequestOption) (*T, error) {
	var out T
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: path, body: body, opts: opts, out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// decode is do for typed JSON methods: a non-JSON (Markdown) body is an error.
func (c *Client) decode(ctx context.Context, cl call) (string, error) {
	text, isJSON, err := c.do(ctx, cl)
	if err != nil {
		return "", err
	}
	if !isJSON {
		return "", &Error{Code: "invalid_response", Message: "got a non-JSON response; for format=markdown use SearchMarkdown or Markdown"}
	}
	return text, nil
}

func (c *Client) markdown(ctx context.Context, path string, body any, opts []RequestOption) (string, error) {
	text, _, err := c.do(ctx, call{method: http.MethodPost, path: path, body: body, opts: opts})
	return text, err
}

// do sends the request with retries. JSON bodies are decoded into cl.out;
// other bodies are returned as text.
func (c *Client) do(ctx context.Context, cl call) (text string, isJSON bool, err error) {
	if c.apiKey == "" {
		return "", false, &Error{Code: "missing_api_key", Message: "no API key: use serpkite.WithAPIKey or set SERPKITE_API_KEY"}
	}
	var o callOpts
	for _, f := range cl.opts {
		f(&o)
	}
	rateLimitOnly := cl.rateLimitOnly && !o.idempotent
	var payload []byte
	if cl.body != nil {
		if payload, err = json.Marshal(cl.body); err != nil {
			return "", false, fmt.Errorf("serpkite: encode request: %w", err)
		}
	}
	for attempt := 0; ; attempt++ {
		canRetry := attempt < c.maxRetries
		status, header, body, err := c.once(ctx, cl.method, cl.path, payload, o.header)
		if err != nil {
			if ctx.Err() != nil {
				return "", false, ctx.Err()
			}
			if canRetry && !rateLimitOnly {
				if err := sleep(ctx, c.backoff(attempt)); err != nil {
					return "", false, err
				}
				continue
			}
			return "", false, err
		}
		if status < 200 || status > 299 {
			retryable := status == http.StatusTooManyRequests || (status >= 500 && !rateLimitOnly)
			if canRetry && retryable {
				if err := sleep(ctx, c.retryDelay(attempt, header)); err != nil {
					return "", false, err
				}
				continue
			}
			return "", false, errorFromResponse(status, header, body)
		}
		if o.info != nil {
			*o.info = responseInfo(status, header)
		}
		mt, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
		if mt == "application/json" || strings.HasSuffix(mt, "+json") {
			if cl.out != nil {
				if err := json.Unmarshal(body, cl.out); err != nil {
					return "", true, &Error{Status: status, Code: "invalid_response", Message: "decode response: " + err.Error(), RequestID: header.Get("X-Request-Id")}
				}
			}
			return string(body), true, nil
		}
		return string(body), false, nil
	}
}

// once performs a single attempt. A transport failure is returned as an *Error
// with Status 0 (code connection_error or timeout).
func (c *Client) once(ctx context.Context, method, path string, payload []byte, extra http.Header) (int, http.Header, []byte, error) {
	actx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		actx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(actx, method, c.baseURL+path, rd)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("serpkite: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json, text/markdown;q=0.9")
	req.Header.Set("User-Agent", "serpkite-go/"+Version)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, transportError(ctx, actx, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, nil, nil, transportError(ctx, actx, err)
	}
	return res.StatusCode, res.Header, body, nil
}

func transportError(ctx, actx context.Context, err error) error {
	if ctx.Err() == nil && errors.Is(actx.Err(), context.DeadlineExceeded) {
		return &Error{Code: "timeout", Message: "request timed out", cause: err}
	}
	return &Error{Code: "connection_error", Message: "request failed: " + err.Error(), cause: err}
}

func (c *Client) backoff(attempt int) time.Duration {
	d := min(c.retryBase<<attempt, retryCap)
	if d <= 0 {
		return 0
	}
	return d/2 + rand.N(d/2+1)
}

func (c *Client) retryDelay(attempt int, h http.Header) time.Duration {
	if d, ok := parseRetryAfter(h.Get("Retry-After"), time.Now()); ok {
		return min(d, retryAfterCap)
	}
	return c.backoff(attempt)
}

// parseRetryAfter reads Retry-After as seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(max(secs, 0) * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func responseInfo(status int, h http.Header) ResponseInfo {
	num := func(name string) (float64, bool) {
		f, err := strconv.ParseFloat(h.Get(name), 64)
		return f, err == nil
	}
	info := ResponseInfo{StatusCode: status, Header: h, RequestID: h.Get("X-Request-Id"), Cache: strings.ToUpper(h.Get("X-Cache"))}
	info.CreditsUsed, _ = num("X-Credits-Used")
	if f, ok := num("X-Credits-Remaining"); ok {
		info.CreditsRemaining = &f
	}
	info.CostUSD, _ = num("X-Cost-USD")
	info.LatencyMs, _ = strconv.Atoi(h.Get("X-Latency-Ms"))
	info.TokensEstimate, _ = strconv.Atoi(h.Get("X-Tokens-Estimate"))
	return info
}

// withField JSON-encodes params into an object and sets key=value.
func withField(params any, key string, value any) (map[string]any, error) {
	m := map[string]any{}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("serpkite: encode request: %w", err)
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("serpkite: params must encode to a JSON object: %w", err)
		}
	}
	m[key] = value
	return m, nil
}
