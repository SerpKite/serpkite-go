package serpkite

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const taskCreated = `{"id":"t1","kind":"crawl","status":"queued","created_at":"2026-10-03T12:00:00Z","poll_url":"https://api.serpkite.com/v1/crawl/t1","credits_reserved":50,"webhook_url":null}`

func TestCrawlLifecycle(t *testing.T) {
	s := newServer(t,
		jsonReply(202, taskCreated, "X-Credits-Reserved", "50"),
		jsonReply(200, `{"id":"t1","kind":"crawl","status":"queued","created_at":"2026-10-03T12:00:00Z","credits_reserved":50,"credits_used":0,"progress":null,"result":null,"error":null}`),
		jsonReply(200, `{"id":"t1","kind":"crawl","status":"running","created_at":"2026-10-03T12:00:00Z","credits_reserved":50,"credits_used":0,"progress":{"pages_done":2,"pages_queued":3,"pages_discovered":5,"limit":50},"result":null,"error":null}`),
		jsonReply(200, `{"id":"t1","kind":"crawl","status":"completed","created_at":"2026-10-03T12:00:00Z","completed_at":"2026-10-03T12:01:00Z","credits_reserved":50,"credits_used":3,"webhook_status":"delivered",
		  "result":{"url":"https://a.com/","pages":[{"url":"https://a.com/","depth":0,"markdown":"# A"}],"failed":[],
		  "stats":{"pages":1,"seconds":4,"discovered":9,"queued":8,"duplicates":1,"sitemap_urls":6,"robots":"found","robots_disallowed":2,"robots_blocked":3,"crawl_delay_ms":500,"stopped":"limit"}},"error":null}`),
		jsonReply(200, `{"id":"t1","status":"canceling"}`))
	c := newTestClient(s)
	ctx := context.Background()
	tk, err := c.Crawl(ctx, CrawlParams{URL: "https://a.com/", Limit: 50, MaxDepth: Int(0), ExcludePaths: []string{"/b$"},
		Sitemap: "only", Query: "pricing", IgnoreQueryParameters: true})
	if err != nil || tk.ID != "t1" || tk.CreditsReserved != 50 || tk.WebhookURL != nil {
		t.Fatalf("crawl %+v %v", tk, err)
	}
	if b := s.requests()[0].Body; b["max_depth"] != float64(0) || b["limit"] != float64(50) || b["sitemap"] != "only" || b["query"] != "pricing" || b["ignore_query_parameters"] != true {
		t.Errorf("body %v", b)
	}
	done, err := c.WaitForCrawl(ctx, "t1", WaitOptions{PollInterval: time.Millisecond, MaxPollInterval: 2 * time.Millisecond})
	if err != nil || !done.Finished() || done.Status != TaskCompleted || done.Result.Pages[0].Markdown != "# A" || done.CreditsUsed != 3 {
		t.Fatalf("wait %+v %v", done, err)
	}
	if st := done.Result.Stats; st.Discovered != 9 || st.Queued != 8 || st.Duplicates != 1 || st.SitemapURLs != 6 || st.Robots != "found" ||
		st.RobotsDisallowed != 2 || st.RobotsBlocked != 3 || st.CrawlDelayMs != 500 || st.Stopped != "limit" {
		t.Errorf("stats %+v", st)
	}
	cancel, err := c.CancelCrawl(ctx, "t1")
	if err != nil || cancel.Status != "canceling" {
		t.Fatalf("cancel %+v %v", cancel, err)
	}
	r := s.requests()
	if len(r) != 5 || r[1].Method != http.MethodGet || r[1].Path != "/v1/crawl/t1" || r[4].Method != http.MethodDelete {
		t.Fatalf("requests %+v", r)
	}
}

func TestCrawlCreateNotRetriedOn5xx(t *testing.T) {
	s := newServer(t, errReply(500, "internal", "boom"))
	if _, err := newTestClient(s).Crawl(context.Background(), CrawlParams{URL: "https://a.com/"}); err == nil {
		t.Fatal("want error")
	}
	if n := len(s.requests()); n != 1 {
		t.Fatalf("POST /v1/crawl was sent %d times, want 1", n)
	}
}

func TestWaitForCrawlFailedTimeoutAndContext(t *testing.T) {
	failed := jsonReply(200, `{"id":"t1","kind":"crawl","status":"failed","created_at":"2026-10-03T12:00:00Z","credits_reserved":25,"credits_used":0,"error":{"code":"no_pages","message":"nothing read"}}`)
	got, err := newTestClient(newServer(t, failed)).WaitForCrawl(context.Background(), "t1")
	if err != nil || got.Status != TaskFailed || got.Error == nil || got.Error.Code != "no_pages" {
		t.Fatalf("failed: %+v %v", got, err)
	}
	running := jsonReply(200, `{"id":"t1","kind":"crawl","status":"running","created_at":"2026-10-03T12:00:00Z","credits_reserved":25,"credits_used":0}`)
	c := newTestClient(newServer(t, running))
	got, err = c.WaitForCrawl(context.Background(), "t1", WaitOptions{Timeout: 20 * time.Millisecond, PollInterval: 5 * time.Millisecond})
	var e *Error
	if !errors.As(err, &e) || e.Code != "timeout" || got == nil || got.Status != TaskRunning {
		t.Fatalf("timeout: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.WaitForCrawl(ctx, "t1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ctx: %v", err)
	}
	if _, err := c.WaitForCrawl(context.Background(), ""); err == nil {
		t.Fatal("empty id")
	}
}

const monitorJSON = `{"id":"m1","name":"agents news","endpoint":"news","request":{"q":"ai agents"},"interval_seconds":3600,
  "webhook_url":"https://example.com/hook","active":true,"next_run_at":"2026-10-03T12:00:00Z","last_run_at":null,"last_status":null,
  "last_error":null,"last_new_results":0,"runs":0,"consecutive_failures":0,"credits_used":0,"created_at":"2026-10-03T12:00:00Z"}`

func TestMonitors(t *testing.T) {
	s := newServer(t,
		jsonReply(201, monitorJSON),
		jsonReply(200, `{"results":[`+monitorJSON+`]}`),
		jsonReply(200, monitorJSON),
		jsonReply(200, monitorJSON),
		jsonReply(202, monitorJSON),
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
		errReply(404, "not_found", "monitor not found"))
	c := newTestClient(s)
	ctx := context.Background()
	m, err := c.Monitors.Create(ctx, MonitorCreateParams{Q: "ai agents", Endpoint: "news", Interval: IntervalHourly, Name: "agents news", WebhookURL: "https://example.com/hook"})
	if err != nil || m.ID != "m1" || m.Request.Q != "ai agents" || m.IntervalSeconds != 3600 {
		t.Fatalf("create %+v %v", m, err)
	}
	list, err := c.Monitors.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	if _, err := c.Monitors.Get(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Monitors.Update(ctx, "m1", MonitorUpdateParams{Active: Bool(false), Interval: IntervalDaily, Name: String("n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Monitors.Run(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Monitors.Delete(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	var e *Error
	if _, err := c.Monitors.Get(ctx, "m1"); !errors.As(err, &e) || e.Status != 404 {
		t.Fatalf("get deleted: %v", err)
	}
	r := s.requests()
	want := []string{"POST /v1/monitors", "GET /v1/monitors", "GET /v1/monitors/m1", "PATCH /v1/monitors/m1", "POST /v1/monitors/m1/run", "DELETE /v1/monitors/m1", "GET /v1/monitors/m1"}
	for i, w := range want {
		if got := r[i].Method + " " + r[i].Path; got != w {
			t.Errorf("request %d = %s, want %s", i, got, w)
		}
	}
	if b := r[3].Body; b["active"] != false || b["interval"] != "daily" || b["name"] != "n" || len(b) != 3 {
		t.Errorf("patch body %v", b)
	}
	if m.WebhookURL == nil || *m.WebhookURL != "https://example.com/hook" {
		t.Errorf("webhook_url %v", m.WebhookURL)
	}
}

func TestMonitorPollAndUpdateSearch(t *testing.T) {
	poll := `{"id":"m2","name":"n","endpoint":"search","request":{"q":"x"},"interval_seconds":86400,"webhook_url":null,"active":false,
	  "next_run_at":"2026-10-03T12:00:00Z","runs":12,"consecutive_failures":10,"last_status":"paused","credits_used":12,"created_at":"2026-10-03T12:00:00Z"}`
	s := newServer(t, jsonReply(201, poll), jsonReply(200, poll), jsonReply(200, poll))
	c := newTestClient(s)
	ctx := context.Background()
	m, err := c.Monitors.Create(ctx, MonitorCreateParams{Q: "x", Active: Bool(false)})
	if err != nil || m.WebhookURL != nil || m.ConsecutiveFailures != 10 || m.Active {
		t.Fatalf("create %+v %v", m, err)
	}
	if _, err := c.Monitors.Update(ctx, "m2", MonitorUpdateParams{Q: "y", Endpoint: "news", Country: "de", Num: 20,
		IncludeDomains: []string{"a.com"}, Engine: Engine{ProviderGoogle}, WebhookURL: String("")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Monitors.Update(ctx, "m2", MonitorUpdateParams{Active: Bool(true)}); err != nil {
		t.Fatal(err)
	}
	r := s.requests()
	if b := r[0].Body; b["active"] != false || len(b) != 2 {
		t.Errorf("create body %v (webhook_url must be omitted)", b)
	}
	if b := r[1].Body; b["q"] != "y" || b["endpoint"] != "news" || b["country"] != "de" || b["num"] != float64(20) || b["webhook_url"] != "" ||
		len(b["include_domains"].([]any)) != 1 || b["engine"] != ProviderGoogle || len(b) != 7 {
		t.Errorf("patch body %v", b)
	}
	if b := r[2].Body; b["active"] != true || len(b) != 1 {
		t.Errorf("resume body %v", b)
	}
}

func TestWebpageMonitorMetadata(t *testing.T) {
	page := `{"id":"m3","name":"pricing","endpoint":"webpage","request":{"url":"https://a.com/pricing","country":"de"},"metadata":{"team":"growth","n":2},
	  "interval_seconds":86400,"webhook_url":null,"active":true,"next_run_at":"2026-10-03T12:00:00Z","runs":0,"consecutive_failures":0,"credits_used":0,"created_at":"2026-10-03T12:00:00Z"}`
	cleared := strings.Replace(page, `"metadata":{"team":"growth","n":2}`, `"metadata":null`, 1)
	s := newServer(t, jsonReply(201, page), jsonReply(200, page), jsonReply(200, cleared),
		jsonReply(200, `{"results":[{"id":"r1","status":"ok","new_results":1,"credits_used":1,"webhook_status":"none","created_at":"2026-10-03T12:00:00Z",
		  "results":[{"url":"https://a.com/pricing","title":"Pricing","change":"changed","content_hash":"h1","markdown":"# Pricing"}]}],"next_before":null}`))
	c := newTestClient(s)
	ctx := context.Background()
	m, err := c.Monitors.Create(ctx, MonitorCreateParams{Endpoint: string(EndpointWebpage), URL: "https://a.com/pricing", Country: "de",
		Metadata: map[string]any{"team": "growth", "n": 2}})
	if err != nil || m.Endpoint != "webpage" || m.Request.URL != "https://a.com/pricing" || m.Request.Q != "" || m.Metadata["team"] != "growth" {
		t.Fatalf("create %+v %v", m, err)
	}
	if _, err := c.Monitors.Update(ctx, "m3", MonitorUpdateParams{URL: "https://a.com/plans", Metadata: json.RawMessage(`{"team":"sales"}`)}); err != nil {
		t.Fatal(err)
	}
	m, err = c.Monitors.Update(ctx, "m3", MonitorUpdateParams{Metadata: ClearMetadata})
	if err != nil || m.Metadata != nil {
		t.Fatalf("clear %+v %v", m, err)
	}
	runs, err := c.Monitors.Runs(ctx, "m3", nil)
	if err != nil {
		t.Fatal(err)
	}
	var pc MonitorPageChange
	if err := json.Unmarshal(runs.Results[0].Results[0], &pc); err != nil || pc.Change != PageChangeChanged || pc.ContentHash != "h1" || pc.Markdown != "# Pricing" {
		t.Fatalf("page change %+v %v", pc, err)
	}
	r := s.requests()
	if b := r[0].Body; b["endpoint"] != "webpage" || b["url"] != "https://a.com/pricing" || b["q"] != nil || b["metadata"].(map[string]any)["n"] != float64(2) || len(b) != 4 {
		t.Errorf("create body %v", b)
	}
	if b := r[1].Body; b["url"] != "https://a.com/plans" || b["metadata"].(map[string]any)["team"] != "sales" || len(b) != 2 {
		t.Errorf("patch body %v", b)
	}
	if b := r[2].Body; len(b) != 1 {
		t.Errorf("clear body %v", b)
	} else if v, ok := b["metadata"]; !ok || v != nil {
		t.Errorf("clear body must send metadata: null, got %v", b)
	}
}

func TestMonitorRuns(t *testing.T) {
	s := newServer(t,
		jsonReply(200, `{"results":[{"id":"r2","status":"ok","error":null,"new_results":1,"results":[{"position":1,"title":"T","link":"https://a.com"}],
		  "credits_used":1,"webhook_status":"none","created_at":"2026-10-03T12:00:00Z"}],"next_before":"r2"}`),
		jsonReply(200, `{"results":[{"id":"r1","status":"error","error":"boom","new_results":0,"results":null,"credits_used":0,"webhook_status":null,"created_at":"2026-10-02T12:00:00Z"}],"next_before":null}`),
		jsonReply(200, `{"results":[],"next_before":null}`))
	c := newTestClient(s)
	ctx := context.Background()
	page, err := c.Monitors.Runs(ctx, "m1", &MonitorRunsParams{Limit: 1})
	if err != nil || len(page.Results) != 1 || page.NextBefore == nil || *page.NextBefore != "r2" || *page.Results[0].WebhookStatus != "none" {
		t.Fatalf("page 1 %+v %v", page, err)
	}
	var o OrganicResult
	if err := json.Unmarshal(page.Results[0].Results[0], &o); err != nil || o.Link != "https://a.com" {
		t.Fatalf("result %+v %v", o, err)
	}
	page, err = c.Monitors.Runs(ctx, "m1", &MonitorRunsParams{Limit: 1, Before: *page.NextBefore})
	if err != nil || page.NextBefore != nil || page.Results[0].Results != nil || *page.Results[0].Error != "boom" || page.Results[0].WebhookStatus != nil {
		t.Fatalf("page 2 %+v %v", page, err)
	}
	if _, err := c.Monitors.Runs(ctx, "m1", nil); err != nil {
		t.Fatal(err)
	}
	r := s.requests()
	if r[0].Method != http.MethodGet || r[0].Path != "/v1/monitors/m1/runs" || r[0].Query != "limit=1" || r[1].Query != "before=r2&limit=1" || r[2].Query != "" {
		t.Fatalf("requests %+v", r)
	}
}
