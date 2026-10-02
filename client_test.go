package serpkite

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type seen struct {
	Method, Path, Auth, ContentType, UA string
	Body                                map[string]any
}

// server replies with the handlers in order (the last one repeats) and records requests.
type server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []seen
}

func newServer(t *testing.T, handlers ...http.HandlerFunc) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		s.mu.Lock()
		s.reqs = append(s.reqs, seen{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.Header.Get("User-Agent"), body})
		i := min(len(s.reqs)-1, len(handlers)-1)
		s.mu.Unlock()
		handlers[i](w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) requests() []seen {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slicesClone(s.reqs)
}

func slicesClone[T any](v []T) []T { return append([]T(nil), v...) }

func jsonReply(status int, body string, hdr ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i+1 < len(hdr); i += 2 {
			w.Header().Set(hdr[i], hdr[i+1])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func errReply(status int, code, msg string, hdr ...string) http.HandlerFunc {
	b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": msg, "request_id": "req_err"}})
	return jsonReply(status, string(b), hdr...)
}

const searchJSON = `{
  "request": {"endpoint": "search", "engine": "google", "q": "best espresso", "country": "us"},
  "results": [{"position": 1, "title": "Espresso", "link": "https://example.com/", "domain": "example.com", "displayed_link": "example.com", "snippet": "…"}],
  "ai_overview": {"text": "Espresso is…", "references": [{"link": "https://example.com/", "domain": "example.com"}]},
  "related_searches": [{"query": "espresso machine"}],
  "meta": {"request_id": "req_1", "credits_used": 1, "cached": false, "latency_ms": 812}
}`

func newTestClient(s *server, opts ...Option) *Client {
	return NewClient(append([]Option{WithAPIKey("skt_live_test"), WithBaseURL(s.URL + "/"), WithRetryBackoff(time.Millisecond)}, opts...)...)
}

func TestSearch(t *testing.T) {
	s := newServer(t, jsonReply(200, searchJSON, "X-Credits-Used", "1", "X-Credits-Remaining", "41.5", "X-Cache", "MISS", "X-Request-Id", "req_1", "X-Tokens-Estimate", "321"))
	c := newTestClient(s)
	var info ResponseInfo
	res, err := c.Search(context.Background(), SearchParams{Q: "best espresso", Country: "us", AIOverview: Bool(false), IncludeContent: 2}, CaptureResponse(&info))
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].Title != "Espresso" || res.Results[0].DisplayedLink != "example.com" || res.Meta.CreditsUsed != 1 || res.Meta.LatencyMs != 812 {
		t.Fatalf("bad decode: %+v", res)
	}
	if res.AIOverview == nil || res.AIOverview.References[0].Domain != "example.com" || res.RelatedSearches[0].Query != "espresso machine" {
		t.Fatalf("bad extras: %+v", res)
	}
	r := s.requests()[0]
	if r.Method != "POST" || r.Path != "/v1/search" || r.Auth != "Bearer skt_live_test" || r.ContentType != "application/json" || !strings.HasPrefix(r.UA, "serpkite-go/") {
		t.Fatalf("bad request: %+v", r)
	}
	want := map[string]any{"q": "best espresso", "country": "us", "ai_overview": false, "include_content": float64(2)}
	if len(r.Body) != len(want) {
		t.Fatalf("body = %v, want %v", r.Body, want)
	}
	for k, v := range want {
		if r.Body[k] != v {
			t.Fatalf("body[%s] = %v, want %v", k, r.Body[k], v)
		}
	}
	if info.CreditsUsed != 1 || info.CreditsRemaining == nil || *info.CreditsRemaining != 41.5 || info.Cache != "MISS" || info.RequestID != "req_1" || info.TokensEstimate != 321 {
		t.Fatalf("bad info: %+v", info)
	}
}

func TestEngineAndRoute(t *testing.T) {
	body := `{
  "request": {"endpoint": "search", "engine": ["google", "brave"], "q": "espresso"},
  "results": [],
  "related_searches": [],
  "meta": {"request_id": "req_1", "credits_used": 1, "cached": false, "engine": "brave",
    "route": [{"provider": "google", "outcome": "blocked", "ms": 812}, {"provider": "brave", "outcome": "ok", "ms": 431}]}
}`
	s := newServer(t, jsonReply(200, body))
	c := newTestClient(s)
	res, err := c.Search(context.Background(), SearchParams{Q: "espresso", Engine: Engines(ProviderGoogle, ProviderBrave)})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.requests()[0].Body["engine"]; !reflect.DeepEqual(got, []any{"google", "brave"}) {
		t.Fatalf("engine sent as %#v", got)
	}
	if res.Meta.Engine != "brave" || len(res.Meta.Route) != 2 || res.Meta.Route[0] != (RouteStep{Provider: "google", Outcome: "blocked", Ms: 812}) {
		t.Fatalf("bad meta: %+v", res.Meta)
	}
	if !reflect.DeepEqual(res.Request.Engine, Engine{"google", "brave"}) || res.Request.Engine.String() != "google,brave" {
		t.Fatalf("bad request echo: %#v", res.Request.Engine)
	}

	if _, err := c.News(context.Background(), SearchParams{Q: "espresso", Engine: Engines(EngineAuto)}); err != nil {
		t.Fatal(err)
	}
	if got := s.requests()[1].Body["engine"]; got != "auto" {
		t.Fatalf("engine sent as %#v", got)
	}
	if _, err := c.Search(context.Background(), SearchParams{Q: "espresso"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.requests()[2].Body["engine"]; ok {
		t.Fatal("nil Engine must be omitted")
	}
}

func TestEngineConsensusSources(t *testing.T) {
	body := `{
  "request": {"endpoint": "search", "engine": "consensus", "q": "espresso"},
  "results": [{"position": 1, "title": "Espresso", "link": "https://example.com/", "domain": "example.com", "sources": ["google", "wikipedia"]}],
  "related_searches": [],
  "meta": {"request_id": "req_1", "credits_used": 1.25, "cached": false, "engine": "consensus",
    "route": [{"provider": "google", "outcome": "ok", "ms": 900}, {"provider": "wikipedia", "outcome": "ok", "ms": 300}]}
}`
	s := newServer(t, jsonReply(200, body))
	res, err := newTestClient(s).Search(context.Background(), SearchParams{Q: "espresso", Engine: Engines(EngineConsensus)})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.requests()[0].Body["engine"]; got != "consensus" {
		t.Fatalf("engine sent as %#v", got)
	}
	if res.Meta.Engine != EngineConsensus || !reflect.DeepEqual(res.Results[0].Sources, []string{ProviderGoogle, ProviderWikipedia}) {
		t.Fatalf("bad consensus response: %+v %+v", res.Meta, res.Results[0])
	}
}

func TestEngineJSON(t *testing.T) {
	for in, want := range map[string]Engine{`"google"`: {"google"}, `"google,brave"`: {"google", "brave"}, `["auto"]`: {"auto"}, `null`: nil} {
		var e Engine
		if err := json.Unmarshal([]byte(in), &e); err != nil || !reflect.DeepEqual(e, want) {
			t.Errorf("Unmarshal(%s) = %#v, %v; want %#v", in, e, err, want)
		}
	}
	if _, err := json.Marshal(Engine{}); err != nil {
		t.Fatal(err)
	}
	var e Engine
	if err := json.Unmarshal([]byte(`1`), &e); err == nil {
		t.Fatal("want an error for a number")
	}
}

func TestEnvKeyAndMissingKey(t *testing.T) {
	s := newServer(t, jsonReply(200, searchJSON))
	t.Setenv("SERPKITE_API_KEY", "skt_live_env")
	t.Setenv("SERPKITE_BASE_URL", s.URL)
	if _, err := NewClient().Search(context.Background(), SearchParams{Q: "x"}); err != nil {
		t.Fatal(err)
	}
	if got := s.requests()[0].Auth; got != "Bearer skt_live_env" {
		t.Fatalf("auth = %q", got)
	}
	t.Setenv("SERPKITE_API_KEY", "")
	_, err := NewClient().Search(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Code != "missing_api_key" {
		t.Fatalf("err = %v", err)
	}
	if len(s.requests()) != 1 {
		t.Fatal("a request was sent without a key")
	}
}

func TestEveryPath(t *testing.T) {
	s := newServer(t, jsonReply(200, `{"results": [], "meta": {"request_id": "r", "credits_used": 0, "cached": false}}`))
	c := newTestClient(s)
	ctx := context.Background()
	q := SearchParams{Q: "a"}
	calls := []func() error{
		func() error { _, err := c.Search(ctx, q); return err },
		func() error { _, err := c.Images(ctx, q); return err },
		func() error { _, err := c.Videos(ctx, q); return err },
		func() error { _, err := c.News(ctx, q); return err },
		func() error { _, err := c.Maps(ctx, SearchParams{Q: "a", LL: "@40.7,-74,14z"}); return err },
		func() error { _, err := c.Places(ctx, q); return err },
		func() error { _, err := c.Reviews(ctx, ReviewsParams{PlaceID: "ChIJ", Sort: "newest"}); return err },
		func() error { _, err := c.Shopping(ctx, q); return err },
		func() error { _, err := c.Scholar(ctx, q); return err },
		func() error { _, err := c.Patents(ctx, q); return err },
		func() error { _, err := c.Autocomplete(ctx, q); return err },
		func() error { _, err := c.Lens(ctx, LensParams{URL: "https://example.com/a.jpg"}); return err },
		func() error { _, err := c.AIMode(ctx, q); return err },
		func() error { _, err := c.Webpage(ctx, WebpageParams{URL: "https://example.com"}); return err },
		func() error { _, err := c.Rank(ctx, RankParams{Q: "a", Domain: "example.com", Num: 50}); return err },
		func() error { _, err := c.Account(ctx); return err },
		func() error { _, err := c.Batches.Get(ctx, "b1"); return err },
	}
	for i, f := range calls {
		if err := f(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	want := []string{
		"POST /v1/search", "POST /v1/images", "POST /v1/videos", "POST /v1/news", "POST /v1/maps", "POST /v1/places",
		"POST /v1/reviews", "POST /v1/shopping", "POST /v1/scholar", "POST /v1/patents", "POST /v1/autocomplete",
		"POST /v1/lens", "POST /v1/ai-mode", "POST /v1/webpage", "POST /v1/rank", "GET /v1/account", "GET /v1/batches/b1",
	}
	reqs := s.requests()
	for i, w := range want {
		if got := reqs[i].Method + " " + reqs[i].Path; got != w {
			t.Errorf("call %d: %s, want %s", i, got, w)
		}
	}
	if reqs[6].Body["place_id"] != "ChIJ" || reqs[4].Body["ll"] != "@40.7,-74,14z" {
		t.Errorf("bad bodies: %v %v", reqs[6].Body, reqs[4].Body)
	}
	if reqs[15].ContentType != "" || reqs[15].Body != nil {
		t.Errorf("GET sent a body: %+v", reqs[15])
	}
}

func TestMarkdown(t *testing.T) {
	md := "# best espresso\n\n1. [Espresso](https://example.com/)\n"
	s := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, md)
	})
	c := newTestClient(s)
	ctx := context.Background()
	got, err := c.SearchMarkdown(ctx, SearchParams{Q: "best espresso"})
	if err != nil || got != md {
		t.Fatalf("SearchMarkdown = %q, %v", got, err)
	}
	got, err = c.Markdown(ctx, EndpointNews, SearchParams{Q: "espresso", Country: "de"})
	if err != nil || got != md {
		t.Fatalf("Markdown = %q, %v", got, err)
	}
	page, err := c.Webpage(ctx, WebpageParams{URL: "https://example.com", Format: FormatMarkdown})
	if err != nil || page.Markdown != md || page.URL != "https://example.com" {
		t.Fatalf("Webpage markdown = %+v, %v", page, err)
	}
	// A typed method with format=markdown explains what to use instead.
	_, err = c.Search(ctx, SearchParams{Q: "x", Format: FormatMarkdown})
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_response" {
		t.Fatalf("err = %v", err)
	}
	reqs := s.requests()
	if reqs[0].Body["format"] != "markdown" || reqs[1].Body["format"] != "markdown" || reqs[1].Body["country"] != "de" || reqs[1].Path != "/v1/news" {
		t.Fatalf("bad requests: %+v", reqs)
	}
}

func TestErrorMapping(t *testing.T) {
	s := newServer(t, errReply(402, "insufficient_credits", "balance is 0"))
	_, err := newTestClient(s).Search(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %T %v", err, err)
	}
	if e.Status != 402 || e.Code != "insufficient_credits" || e.Message != "balance is 0" || e.RequestID != "req_err" {
		t.Fatalf("bad error: %+v", e)
	}
	if !strings.Contains(e.Error(), "insufficient_credits") || e.Retryable() {
		t.Fatalf("Error() = %q", e.Error())
	}
	if len(s.requests()) != 1 {
		t.Fatal("4xx was retried")
	}
}

func TestNonJSONError(t *testing.T) {
	s := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "r9")
		w.WriteHeader(502)
		_, _ = io.WriteString(w, "error code: 502") // what Cloudflare sends instead of the origin body
	})
	_, err := newTestClient(s, WithMaxRetries(0)).News(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Status != 502 || e.Code != "upstream_error" || e.Message != "error code: 502" || e.RequestID != "r9" {
		t.Fatalf("err = %+v", err)
	}
}

func TestUpstream503(t *testing.T) {
	s := newServer(t, errReply(503, "upstream_blocked", "retry shortly"))
	_, err := newTestClient(s, WithMaxRetries(0)).Search(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Status != 503 || e.Code != "upstream_blocked" || !e.Retryable() {
		t.Fatalf("err = %+v", err)
	}
}

func TestRetriesLegacy502And504(t *testing.T) {
	s := newServer(t, errReply(502, "upstream_error", "boom"), errReply(504, "upstream_timeout", "slow"), jsonReply(200, searchJSON))
	if _, err := newTestClient(s).Search(context.Background(), SearchParams{Q: "x"}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.requests()); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestRetries(t *testing.T) {
	s := newServer(t, errReply(503, "upstream_blocked", "boom"), errReply(503, "unavailable", "down"), jsonReply(200, searchJSON))
	res, err := newTestClient(s).Search(context.Background(), SearchParams{Q: "x"})
	if err != nil || len(res.Results) != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if n := len(s.requests()); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestRetriesGiveUp(t *testing.T) {
	s := newServer(t, errReply(503, "unavailable", "down"))
	_, err := newTestClient(s, WithMaxRetries(3)).Search(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Code != "unavailable" || !e.Retryable() {
		t.Fatalf("err = %v", err)
	}
	if n := len(s.requests()); n != 4 {
		t.Fatalf("attempts = %d, want 4", n)
	}
}

func TestRetryAfter(t *testing.T) {
	s := newServer(t, errReply(429, "rate_limited", "slow down", "Retry-After", "0.05"), jsonReply(200, searchJSON))
	start := time.Now()
	if _, err := newTestClient(s, WithRetryBackoff(0)).Search(context.Background(), SearchParams{Q: "x"}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 45*time.Millisecond {
		t.Fatalf("Retry-After ignored: %s", d)
	}
	if n := len(s.requests()); n != 2 {
		t.Fatalf("attempts = %d", n)
	}
}

func TestNetworkErrorAndTimeout(t *testing.T) {
	s := newServer(t, jsonReply(200, searchJSON))
	url := s.URL
	s.Close()
	_, err := NewClient(WithAPIKey("k"), WithBaseURL(url), WithMaxRetries(1), WithRetryBackoff(time.Millisecond)).Search(context.Background(), SearchParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Code != "connection_error" || e.Status != 0 {
		t.Fatalf("err = %v", err)
	}

	slow := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	_, err = newTestClient(slow, WithTimeout(30*time.Millisecond), WithMaxRetries(1)).Search(context.Background(), SearchParams{Q: "x"})
	if !errors.As(err, &e) || e.Code != "timeout" {
		t.Fatalf("err = %v", err)
	}
	if n := len(slow.requests()); n != 2 {
		t.Fatalf("timeouts should be retried: %d attempts", n)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = newTestClient(slow).Search(ctx, SearchParams{Q: "x"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

const batchQueued = `{"id": "b1", "status": "queued", "endpoint": "/v1/search", "created_at": "2026-09-29T00:00:00Z", "poll_url": "https://api.serpkite.com/v1/batches/b1", "completed_at": null, "credits_used": 0, "webhook_url": null, "webhook_status": null}`

func TestBatchesCreate(t *testing.T) {
	s := newServer(t, jsonReply(202, `{"batches": [`+batchQueued+`, {"error": {"code": "invalid_request", "message": "q is required", "request_id": "r:1"}}]}`))
	res, err := newTestClient(s).Batches.Create(context.Background(), BatchCreateParams{
		Endpoint:   EndpointSearch,
		Requests:   []any{SearchParams{Q: "a"}, map[string]any{"q": ""}},
		WebhookURL: "https://hooks.example.com/serpkite",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Batches) != 2 || res.Batches[0].Batch == nil || res.Batches[0].Batch.ID != "b1" || res.Batches[0].Err != nil {
		t.Fatalf("entry 0 = %+v", res.Batches[0])
	}
	if e := res.Batches[1].Err; e == nil || e.Code != "invalid_request" || e.RequestID != "r:1" || res.Batches[1].Batch != nil {
		t.Fatalf("entry 1 = %+v", res.Batches[1])
	}
	if q := res.Queued(); len(q) != 1 || q[0].ID != "b1" {
		t.Fatalf("Queued = %+v", q)
	}
	r := s.requests()[0]
	reqs, _ := r.Body["requests"].([]any)
	if r.Path != "/v1/batches" || r.Body["endpoint"] != "search" || len(reqs) != 2 || r.Body["webhook_url"] != "https://hooks.example.com/serpkite" {
		t.Fatalf("bad request: %+v", r)
	}
}

func TestBatchesCreateRetriesOnlyRateLimits(t *testing.T) {
	s := newServer(t, errReply(500, "internal", "boom"), jsonReply(202, `{"batches": []}`))
	_, err := newTestClient(s).Batches.Create(context.Background(), BatchCreateParams{Endpoint: EndpointSearch, Requests: []any{SearchParams{Q: "a"}}})
	var e *Error
	if !errors.As(err, &e) || e.Status != 500 || len(s.requests()) != 1 {
		t.Fatalf("err = %v, attempts = %d", err, len(s.requests()))
	}

	s2 := newServer(t, errReply(429, "rate_limited", "slow", "Retry-After", "0"), jsonReply(202, `{"batches": []}`))
	if _, err := newTestClient(s2).Batches.Create(context.Background(), BatchCreateParams{Endpoint: EndpointSearch, Requests: []any{SearchParams{Q: "a"}}}); err != nil {
		t.Fatal(err)
	}
	if len(s2.requests()) != 2 {
		t.Fatal("429 was not retried")
	}
}

func TestBatchesWait(t *testing.T) {
	running := strings.Replace(batchQueued, `"queued"`, `"running"`, 1)
	done := `{"id": "b1", "status": "done", "endpoint": "/v1/search", "created_at": "2026-09-29T00:00:00Z", "completed_at": "2026-09-29T00:00:05Z", "poll_url": "", "credits_used": 0.5, "result": ` + searchJSON + `}`
	s := newServer(t, jsonReply(200, batchQueued), jsonReply(200, running), jsonReply(200, done))
	b, err := newTestClient(s).Batches.Wait(context.Background(), "b1", WaitOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != BatchDone || b.CreditsUsed != 0.5 || b.CompletedAt == nil {
		t.Fatalf("batch = %+v", b)
	}
	var res SearchResponse
	if ok, err := b.DecodeResult(&res); !ok || err != nil || res.Results[0].Title != "Espresso" {
		t.Fatalf("DecodeResult = %v %v %+v", ok, err, res)
	}
	reqs := s.requests()
	if len(reqs) != 3 || reqs[2].Path != "/v1/batches/b1" || reqs[2].Method != "GET" {
		t.Fatalf("polls = %+v", reqs)
	}
}

func TestBatchesWaitFailedAndTimeout(t *testing.T) {
	failed := `{"id": "b2", "status": "failed", "endpoint": "/v1/search", "created_at": "2026-09-29T00:00:00Z", "poll_url": "", "error": {"code": "upstream_error", "message": "x"}}`
	s := newServer(t, jsonReply(200, failed))
	b, err := newTestClient(s).Batches.Wait(context.Background(), "b2")
	if err != nil || b.Status != BatchFailed || b.Error == nil || b.Error.Code != "upstream_error" {
		t.Fatalf("b = %+v, err = %v", b, err)
	}

	stuck := newServer(t, jsonReply(200, strings.Replace(batchQueued, `"queued"`, `"running"`, 1)))
	_, err = newTestClient(stuck).Batches.Wait(context.Background(), "b1", WaitOptions{PollInterval: 5 * time.Millisecond, Timeout: 30 * time.Millisecond})
	var e *Error
	if !errors.As(err, &e) || e.Code != "timeout" {
		t.Fatalf("err = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = newTestClient(stuck).Batches.Wait(ctx, "b1", WaitOptions{PollInterval: time.Second})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if d, ok := parseRetryAfter("2", now); !ok || d != 2*time.Second {
		t.Fatal(d, ok)
	}
	if d, ok := parseRetryAfter(now.Add(5*time.Second).Format(http.TimeFormat), now); !ok || d != 5*time.Second {
		t.Fatal(d, ok)
	}
	if _, ok := parseRetryAfter("soon", now); ok {
		t.Fatal("parsed garbage")
	}
}

func TestBatchEntryRoundTrip(t *testing.T) {
	in := `{"batches":[{"error":{"code":"invalid_request","message":"m","request_id":"r"}}]}`
	var r BatchCreateResponse
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(r)
	if string(out) != in {
		t.Fatalf("round trip = %s", out)
	}
}
