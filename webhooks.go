package serpkite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WebhookTolerance is how far a delivery's timestamp may be from now in
// [VerifyWebhook].
const WebhookTolerance = 5 * time.Minute

// VerifyWebhook checks a webhook delivery (every event: batch.completed,
// crawl.completed, monitor.results; the event type is in the X-SerpKite-Event
// header): X-SerpKite-Signature must be
// "v1=" + hex(HMAC-SHA256(secret, "<X-SerpKite-Timestamp>.<raw body>")) and
// the timestamp within [WebhookTolerance] of now. Pass the raw request body
// exactly as received.
//
//	body, _ := io.ReadAll(r.Body)
//	if !serpkite.VerifyWebhook(secret, body, r.Header, time.Now()) { w.WriteHeader(401); return }
func VerifyWebhook(secret string, body []byte, h http.Header, now time.Time) bool {
	sig, ok := strings.CutPrefix(h.Get("X-SerpKite-Signature"), "v1=")
	ts := h.Get("X-SerpKite-Timestamp")
	unix, err := strconv.ParseInt(ts, 10, 64)
	if secret == "" || !ok || err != nil {
		return false
	}
	if d := now.Sub(time.Unix(unix, 0)); d > WebhookTolerance || d < -WebhookTolerance {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts))
	m.Write([]byte("."))
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}
