package serpkite

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const citationJSON = `{"index":1,"title":"RAG","link":"https://e.com/rag","domain":"e.com"}`

func sseReply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req_s")
		w.Header().Set("X-Credits-Used", "1")
		w.WriteHeader(200)
		f, _ := w.(http.Flusher)
		// Deliver in small pieces, split mid-line, to exercise the scanner.
		for i := 0; i < len(body); i += 7 {
			_, _ = io.WriteString(w, body[i:min(i+7, len(body))])
			if f != nil {
				f.Flush()
			}
		}
	}
}

func TestAnswer(t *testing.T) {
	s := newServer(t, jsonReply(200, `{"request":{"endpoint":"answer","engine":"google","q":"what is rag"},
	  "answer":"Retrieval-augmented generation [1].","output":{"ok":true},"citations":[`+citationJSON+`],"results":[`+citationJSON+`],
	  "meta":{"request_id":"r","credits_used":8,"cached":false,"credits":{"search":4,"answer":4},"llm":{"provider":"anthropic","model":"claude-haiku-5-5","input_tokens":900}}}`))
	res, err := newTestClient(s).Answer(context.Background(), AnswerParams{Q: "what is rag", Depth: DepthStandard, NumSources: 3, Stream: true,
		OutputSchema: map[string]any{"type": "object"}})
	if err != nil {
		t.Fatal(err)
	}
	b := s.requests()[0].Body
	if s.requests()[0].Path != "/v1/answer" || b["q"] != "what is rag" || b["depth"] != "standard" || b["num_sources"] != float64(3) || b["stream"] != nil || b["output_schema"] == nil {
		t.Errorf("request %+v", s.requests()[0])
	}
	if res.Text() != "Retrieval-augmented generation [1]." || res.Citations[0].Index != 1 || res.Meta.Credits.Answer != 4 || res.Meta.CreditsUsed != 8 || res.Meta.LLM.Provider != "anthropic" {
		t.Errorf("response %+v", res)
	}
	var out struct{ OK bool }
	if ok, err := res.DecodeOutput(&out); !ok || err != nil || !out.OK {
		t.Errorf("output %v %v %+v", ok, err, out)
	}
}

func TestAnswerAIUnavailable(t *testing.T) {
	s := newServer(t, errReply(503, "ai_unavailable", "AI is off", "Retry-After", "0"))
	_, err := newTestClient(s, WithMaxRetries(0)).Answer(context.Background(), AnswerParams{Q: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Status != 503 || e.Code != "ai_unavailable" {
		t.Fatalf("err %v", err)
	}
}

func TestAnswerNotRetriedOn5xx(t *testing.T) {
	// A 503 after the search still charged it: a retry would bill the search again.
	s := newServer(t, errReply(503, "ai_unavailable", "AI is off", "Retry-After", "0"))
	if _, err := newTestClient(s).Answer(context.Background(), AnswerParams{Q: "x"}); err == nil {
		t.Fatal("want an error")
	}
	if n := len(s.requests()); n != 1 {
		t.Fatalf("answer sent %d times", n)
	}
}

func TestAnswerStream(t *testing.T) {
	stream := ": keep-alive\n\n" +
		"event: results\ndata: {\"request\":{\"endpoint\":\"answer\",\"engine\":\"google\"},\"results\":[" + citationJSON + "]}\n\n" +
		"event: delta\ndata: {\"text\":\"A \"}\n\n" +
		"event: unknown\ndata: {}\n\n" +
		"event: delta\r\ndata: {\"text\":\"[1].\"}\r\n\r\n" +
		"event: done\ndata: {\"answer\":\"A [1].\",\"citations\":[" + citationJSON + "],\"meta\":{\"request_id\":\"r\",\"credits_used\":5,\"cached\":false}}\n\n"
	s := newServer(t, errReply(503, "ai_unavailable", "try again"), sseReply(stream))
	var info ResponseInfo
	st, err := newTestClient(s).AnswerStream(context.Background(), AnswerParams{Q: "what is rag", Depth: DepthFast}, CaptureResponse(&info))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var types []string
	var text strings.Builder
	for st.Next() {
		ev := st.Event()
		types = append(types, ev.Type)
		if ev.Delta != nil {
			text.WriteString(ev.Delta.Text)
		}
	}
	if err := st.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "results,delta,delta,done" || text.String() != "A [1]." {
		t.Errorf("events %v %q", types, text.String())
	}
	if ev := st.Event(); ev.Done == nil || ev.Done.Meta.CreditsUsed != 5 || ev.Done.Citations[0].Link != "https://e.com/rag" {
		t.Errorf("done %+v", ev.Done)
	}
	r := s.requests()
	if len(r) != 2 || r[1].Body["stream"] != true || r[1].Body["depth"] != "fast" || !strings.HasPrefix(r[1].UA, "serpkite-go/") {
		t.Errorf("requests %+v", r)
	}
	if info.CreditsUsed != 1 || info.RequestID != "req_s" {
		t.Errorf("info %+v", info)
	}
}

func TestAnswerStreamErrors(t *testing.T) {
	ctx := context.Background()
	// An error event ends the stream with an *Error.
	errStream := "event: results\ndata: {\"request\":{},\"results\":[]}\n\n" +
		"event: error\ndata: {\"error\":{\"code\":\"ai_unavailable\",\"message\":\"answer generation failed\",\"request_id\":\"req_9\"}}\n\n"
	st, err := newTestClient(newServer(t, sseReply(errStream))).AnswerStream(ctx, AnswerParams{Q: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Collect(); err == nil {
		t.Fatal("want error")
	} else if e, ok := err.(*Error); !ok || e.Code != "ai_unavailable" || e.Status != 503 || e.RequestID != "req_9" {
		t.Fatalf("err %v", err)
	}

	// A stream cut before done.
	st, err = newTestClient(newServer(t, sseReply("event: delta\ndata: {\"text\":\"A\"}\n\n"))).AnswerStream(ctx, AnswerParams{Q: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Collect(); err == nil || err.(*Error).Code != "invalid_response" {
		t.Fatalf("truncated: %v", err)
	}

	// 4xx before the stream starts is returned by AnswerStream, not retried.
	s := newServer(t, errReply(400, "invalid_request", "stream can't take output_schema"))
	if _, err := newTestClient(s).AnswerStream(ctx, AnswerParams{Q: "x"}); err == nil || err.(*Error).Code != "invalid_request" {
		t.Fatalf("400: %v", err)
	}
	if n := len(s.requests()); n != 1 {
		t.Errorf("400 sent %d times", n)
	}

	// A plain JSON answer is replayed as results + done.
	s = newServer(t, jsonReply(200, `{"request":{"endpoint":"answer","engine":"google"},"answer":null,"citations":[],"results":[],"meta":{"request_id":"r","credits_used":1,"cached":false,"warnings":["no results"]}}`))
	st, err = newTestClient(s).AnswerStream(ctx, AnswerParams{Q: "zzz"})
	if err != nil {
		t.Fatal(err)
	}
	done, err := st.Collect()
	if err != nil || done.Answer != "" || len(done.Meta.Warnings) != 1 {
		t.Fatalf("json fallback %+v %v", done, err)
	}

	// The client timeout covers connecting only, not a slow stream.
	slow := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(60 * time.Millisecond)
		_, _ = io.WriteString(w, "event: done\ndata: {\"answer\":\"ok\",\"citations\":[],\"meta\":{\"request_id\":\"r\",\"credits_used\":1,\"cached\":false}}\n\n")
	}
	st, err = newTestClient(newServer(t, slow), WithTimeout(30*time.Millisecond)).AnswerStream(ctx, AnswerParams{Q: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if done, err := st.Collect(); err != nil || done.Answer != "ok" {
		t.Fatalf("slow stream %+v %v", done, err)
	}
}

func TestResearchLifecycle(t *testing.T) {
	created := `{"id":"r1","kind":"research","status":"queued","created_at":"2026-10-24T12:00:00Z","poll_url":"https://api.serpkite.com/v1/research/r1","credits_reserved":75,"webhook_url":null}`
	s := newServer(t,
		errReply(503, "unavailable", "down"),
		jsonReply(202, created),
		jsonReply(200, `{"id":"r1","kind":"research","status":"running","created_at":"2026-10-24T12:00:00Z","credits_reserved":75,"credits_used":0,"progress":{"round":1,"rounds":2,"queries":5,"sources":12,"step":"searching"}}`),
		jsonReply(200, `{"id":"r1","kind":"research","status":"completed","created_at":"2026-10-24T12:00:00Z","credits_reserved":75,"credits_used":75,
		  "result":{"input":"vector dbs","effort":"standard","report":"# Report [1]","citations":[`+citationJSON+`],
		  "sources":[{"index":1,"title":"RAG","link":"https://e.com/rag","query":"vector databases"}],"queries":["vector databases"],
		  "stats":{"searches":5,"sources":12,"rounds":2},"output":{"winner":"pgvector"}}}`),
		jsonReply(200, `{"id":"r1","status":"canceled"}`))
	c := newTestClient(s)
	ctx := context.Background()
	if _, err := c.Research(ctx, ResearchParams{Input: "vector dbs"}); err == nil {
		t.Fatal("503 must not be retried")
	}
	tk, err := c.Research(ctx, ResearchParams{Input: "vector dbs", Effort: EffortStandard, IncludeDomains: []string{"github.com"}, WebhookURL: "https://example.com/hook"})
	if err != nil || tk.Kind != "research" || tk.ID != "r1" {
		t.Fatalf("research %+v %v", tk, err)
	}
	if b := s.requests()[1].Body; b["input"] != "vector dbs" || b["effort"] != "standard" || b["webhook_url"] != "https://example.com/hook" {
		t.Errorf("body %v", b)
	}
	got, err := c.GetResearch(ctx, "r1")
	if err != nil || got.Progress == nil || got.Progress.Step != "searching" || got.Progress.Sources != 12 {
		t.Fatalf("get %+v %v", got, err)
	}
	done, err := c.WaitResearch(ctx, "r1", WaitOptions{PollInterval: time.Millisecond})
	if err != nil || !done.Finished() || done.Result == nil || done.Result.Report != "# Report [1]" || done.Result.Sources[0].Query != "vector databases" || done.Result.Stats.Rounds != 2 {
		t.Fatalf("wait %+v %v", done, err)
	}
	var out struct{ Winner string }
	if ok, err := done.Result.DecodeOutput(&out); !ok || err != nil || out.Winner != "pgvector" {
		t.Errorf("output %v %v %+v", ok, err, out)
	}
	if cn, err := c.CancelResearch(ctx, "r1"); err != nil || cn.Status != "canceled" {
		t.Fatalf("cancel %+v %v", cn, err)
	}
	r := s.requests()
	if len(r) != 5 || r[2].Path != "/v1/research/r1" || r[4].Method != http.MethodDelete {
		t.Fatalf("requests %+v", r)
	}

	running := jsonReply(200, `{"id":"r1","kind":"research","status":"running","created_at":"2026-10-24T12:00:00Z","credits_reserved":75,"credits_used":0}`)
	_, err = newTestClient(newServer(t, running)).WaitResearch(ctx, "r1", WaitOptions{Timeout: 20 * time.Millisecond, PollInterval: 5 * time.Millisecond})
	var e *Error
	if !errors.As(err, &e) || e.Code != "timeout" {
		t.Fatalf("timeout: %v", err)
	}
}

func TestIncludeAnswerAndExtractAI(t *testing.T) {
	s := newServer(t,
		jsonReply(200, `{"request":{"endpoint":"search","engine":"google","q":"rag","include_answer":true},"results":[],"related_searches":[],
		  "answer":{"text":"RAG [1].","citations":[`+citationJSON+`]},"meta":{"request_id":"r","credits_used":3,"cached":false,"llm":{"provider":"openai","model":"gpt-6-luna"}}}`),
		jsonReply(200, `{"request":{"endpoint":"news","engine":"google"},"results":[],"meta":{"request_id":"r","credits_used":1,"cached":false,"warnings":["answer failed"]}}`),
		jsonReply(200, `{"request":{"endpoint":"extract"},"results":[{"url":"https://a.com","cached":false,"summary":"About A.","output":{"price":10}},
		  {"url":"https://b.com","cached":false,"summary":null,"summary_error":"failed","output":null,"output_error":"failed"}],"failed":[],
		  "meta":{"request_id":"r","credits_used":7,"succeeded":2,"failed":0,"llm":{"provider":"anthropic"}}}`))
	c := newTestClient(s, WithTimeout(time.Second))
	ctx := context.Background()
	res, err := c.Search(ctx, SearchParams{Q: "rag", IncludeAnswer: true})
	if err != nil || res.Answer == nil || res.Answer.Citations[0].Index != 1 || !res.Request.IncludeAnswer || res.Meta.LLM.Model != "gpt-6-luna" {
		t.Fatalf("search %+v %v", res, err)
	}
	news, err := c.News(ctx, SearchParams{Q: "rag", IncludeAnswer: true})
	if err != nil || news.Answer != nil || len(news.Meta.Warnings) != 1 {
		t.Fatalf("news %+v %v", news, err)
	}
	ex, err := c.Extract(ctx, ExtractParams{URLs: []string{"https://a.com", "https://b.com"}, Summary: true,
		OutputSchema: map[string]any{"type": "object"}, Prompt: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.requests()[2].Body; b["summary"] != true || b["prompt"] != "USD" || b["output_schema"] == nil {
		t.Errorf("extract body %v", b)
	}
	var out struct{ Price int }
	if ok, err := ex.Results[0].DecodeOutput(&out); !ok || err != nil || out.Price != 10 || *ex.Results[0].Summary != "About A." {
		t.Errorf("result 0 %+v", ex.Results[0])
	}
	if ok, _ := ex.Results[1].DecodeOutput(&out); ok || ex.Results[1].Summary != nil || ex.Results[1].OutputError != "failed" || ex.Meta.LLM.Provider != "anthropic" {
		t.Errorf("result 1 %+v", ex.Results[1])
	}
	if b := s.requests()[0].Body; b["include_answer"] != true {
		t.Errorf("search body %v", b)
	}
}

func TestResearchWebhook(t *testing.T) {
	ev := TaskCompletedEvent{Result: []byte(`{"input":"x","effort":"lite","report":"# R","citations":[],"sources":[],"queries":["x"]}`)}
	var r ResearchResult
	if ok, err := ev.DecodeResult(&r); !ok || err != nil || r.Report != "# R" {
		t.Fatalf("decode %v %v %+v", ok, err, r)
	}
	if EventResearchCompleted != "research.completed" {
		t.Fatal(EventResearchCompleted)
	}
}
