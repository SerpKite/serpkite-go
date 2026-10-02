package serpkite

import (
	"context"
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
