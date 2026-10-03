package serpkite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestVerifyWebhook(t *testing.T) {
	// Same vector as SignWebhook in backend/cmd/serp-api.
	secret, body := "whsec_test", []byte(`{"event":"batch.completed"}`)
	h := http.Header{}
	h.Set("X-SerpKite-Signature", "v1=30e487bd0bd03db9643edcae3622a5d5cd3ee3f108cd4c9130e7670f518aeed1")
	h.Set("X-SerpKite-Timestamp", "1700000000")
	at := time.Unix(1700000060, 0)
	if !VerifyWebhook(secret, body, h, at) {
		t.Fatal("valid delivery rejected")
	}
	if VerifyWebhook(secret, append(body, ' '), h, at) || VerifyWebhook("whsec_other", body, h, at) ||
		VerifyWebhook(secret, body, h, time.Unix(1700000301, 0)) {
		t.Fatal("tampered, wrong-secret or stale delivery accepted")
	}
	h.Del("X-SerpKite-Signature")
	if VerifyWebhook(secret, body, h, at) {
		t.Fatal("unsigned delivery accepted")
	}
}

func TestBatchesCreateWithIdempotencyKeyRetries5xx(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	record := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			keys = append(keys, r.Header.Get("Idempotency-Key"))
			mu.Unlock()
			next(w, r)
		}
	}
	s := newServer(t, record(errReply(500, "internal", "boom")), record(jsonReply(202, `{"batches": []}`)))
	_, err := newTestClient(s).Batches.Create(context.Background(),
		BatchCreateParams{Endpoint: EndpointSearch, Requests: []any{SearchParams{Q: "a"}}}, WithIdempotencyKey("job-42"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "job-42" || keys[1] != "job-42" {
		t.Fatalf("keys = %v", keys)
	}
}

func TestVerifyWebhookTaskAndMonitorEvents(t *testing.T) {
	secret := "whsec_test"
	now := time.Unix(1700000000, 0)
	sign := func(body []byte) http.Header {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte("1700000000." + string(body)))
		h := http.Header{}
		h.Set("X-SerpKite-Timestamp", "1700000000")
		h.Set("X-SerpKite-Signature", "v1="+hex.EncodeToString(m.Sum(nil)))
		return h
	}
	crawl := []byte(`{"event":"crawl.completed","id":"t1","kind":"crawl","status":"completed","created_at":"2026-10-03T12:00:00Z","completed_at":"2026-10-03T12:01:00Z","credits_used":3,"poll_url":"https://api.serpkite.com/v1/crawl/t1","result":{"url":"https://a.com/","pages":[{"url":"https://a.com/","depth":0}],"failed":[],"stats":{"pages":1}}}`)
	h := sign(crawl)
	h.Set("X-SerpKite-Event", EventCrawlCompleted)
	if !VerifyWebhook(secret, crawl, h, now) {
		t.Fatal("crawl.completed rejected")
	}
	var ev TaskCompletedEvent
	if err := json.Unmarshal(crawl, &ev); err != nil || ev.Event != EventCrawlCompleted {
		t.Fatal(err)
	}
	if ev.PollURL != "https://api.serpkite.com/v1/crawl/t1" || ev.ResultOmitted {
		t.Fatalf("crawl event %+v", ev)
	}
	var res CrawlResult
	if ok, err := ev.DecodeResult(&res); !ok || err != nil || res.Pages[0].URL != "https://a.com/" {
		t.Fatalf("decode %v %v %+v", ok, err, res)
	}
	failed := []byte(`{"event":"crawl.completed","id":"t3","kind":"crawl","status":"failed","created_at":"2026-10-03T12:00:00Z","credits_used":0,"result":null,"error":{"code":"no_pages","message":"x"}}`)
	var fev TaskCompletedEvent
	if !VerifyWebhook(secret, failed, sign(failed), now) || json.Unmarshal(failed, &fev) != nil || fev.Error == nil || fev.Error.Code != "no_pages" {
		t.Fatalf("failed crawl event %+v", fev)
	}
	big := []byte(`{"event":"crawl.completed","id":"t2","kind":"crawl","status":"completed","created_at":"2026-10-03T12:00:00Z","credits_used":900,"poll_url":"https://api.serpkite.com/v1/crawl/t2","result":null,"result_omitted":true}`)
	var bev TaskCompletedEvent
	if err := json.Unmarshal(big, &bev); err != nil || !bev.ResultOmitted {
		t.Fatalf("omitted %+v %v", bev, err)
	}
	if ok, err := bev.DecodeResult(&res); ok || err != nil {
		t.Fatalf("omitted result decoded: %v %v", ok, err)
	}
	mon := []byte(`{"event":"monitor.results","monitor_id":"m1","run_id":"r1","name":"n","endpoint":"news","q":"ai","first_run":true,"run_at":"2026-10-03T12:00:00Z","new_results":[{"position":1,"title":"T","link":"https://a.com"}],"credits_used":1}`)
	if !VerifyWebhook(secret, mon, sign(mon), now) {
		t.Fatal("monitor.results rejected")
	}
	var me MonitorResultsEvent
	if err := json.Unmarshal(mon, &me); err != nil || !me.FirstRun || me.RunID != "r1" || len(me.NewResults) != 1 {
		t.Fatalf("monitor event %+v %v", me, err)
	}
	var n NewsResult
	if err := json.Unmarshal(me.NewResults[0], &n); err != nil || n.Link != "https://a.com" {
		t.Fatal(err)
	}
	if me.URL != "" || me.Metadata != nil {
		t.Fatalf("search event url/metadata %+v", me)
	}
	page := []byte(`{"event":"monitor.results","monitor_id":"m2","run_id":"r2","name":"p","endpoint":"webpage","q":"","url":"https://a.com/pricing",
	  "metadata":{"team":"growth"},"first_run":false,"run_at":"2026-10-03T12:00:00Z","new_results":[{"url":"https://a.com/pricing","change":"changed","content_hash":"h","markdown":"# P"}],"credits_used":1}`)
	var pe MonitorResultsEvent
	if err := json.Unmarshal(page, &pe); err != nil || pe.URL != "https://a.com/pricing" || pe.Metadata["team"] != "growth" {
		t.Fatalf("webpage event %+v %v", pe, err)
	}
	var pc MonitorPageChange
	if err := json.Unmarshal(pe.NewResults[0], &pc); err != nil || pc.Change != PageChangeChanged {
		t.Fatalf("page change %+v %v", pc, err)
	}
}
