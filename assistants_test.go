package serpkite

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

const assistantJSON = `{"request":{"endpoint":"chatgpt","prompt":"best CRM","country":"us","web_search":true},
  "text":"HubSpot and Pipedrive [1].","markdown":"HubSpot and Pipedrive [1](https://www.hubspot.com/).","model":"gpt-5-mini",
  "sources":[{"position":1,"title":"HubSpot","link":"https://www.hubspot.com/","domain":"hubspot.com","cited":true},
             {"position":2,"link":"https://g2.com/","domain":"g2.com","cited":false}],
  "search_queries":["best crm for startups"],
  "products":[{"position":1,"title":"Pipedrive","price":"$14","extracted_price":14}],
  "mentions":[{"term":"hubspot.com","mentioned":true,"cited":true,"sources":[1],"text_position":0},
              {"term":"Salesforce","mentioned":false,"cited":false,"sources":[],"text_position":-1}],
  "meta":{"request_id":"r","credits_used":5,"cached":false}}`

func TestChatGPT(t *testing.T) {
	s := newServer(t, jsonReply(200, assistantJSON))
	res, err := newTestClient(s).ChatGPT(context.Background(), ChatGPTParams{Prompt: "best CRM", WebSearch: Bool(false), Country: "us", Track: []string{"hubspot.com", "Salesforce"}})
	if err != nil {
		t.Fatal(err)
	}
	r := s.requests()[0]
	if r.Path != "/v1/chatgpt" || r.Body["prompt"] != "best CRM" || r.Body["web_search"] != false || len(r.Body["track"].([]any)) != 2 || r.Body["include_html"] != nil {
		t.Errorf("request %+v", r)
	}
	if res.Request.Prompt != "best CRM" || !*res.Request.WebSearch || res.Model != "gpt-5-mini" || !res.Sources[0].Cited || res.Sources[1].Cited ||
		res.Products[0].ExtractedPrice != 14 || res.SearchQueries[0] != "best crm for startups" || res.Meta.CreditsUsed != 5 {
		t.Errorf("response %+v", res)
	}
	if m := res.Mentions; len(m) != 2 || m[0].Sources[0] != 1 || m[1].TextPosition != -1 || m[1].Mentioned {
		t.Errorf("mentions %+v", m)
	}
}

func TestAssistantPaths(t *testing.T) {
	s := newServer(t, jsonReply(200, assistantJSON))
	c := newTestClient(s)
	ctx := context.Background()
	p := AssistantParams{Prompt: "p", Language: "de", IncludeHTML: true}
	for _, f := range []func(context.Context, AssistantParams, ...RequestOption) (*AssistantResponse, error){c.Gemini, c.Copilot, c.AIMode, c.BraveAI} {
		if _, err := f(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"/v1/gemini", "/v1/copilot", "/v1/ai_mode", "/v1/brave_ai"}
	for i, r := range s.requests() {
		if r.Path != want[i] || r.Body["language"] != "de" || r.Body["include_html"] != true {
			t.Errorf("request %d %+v", i, r)
		}
	}
}

func TestAssistantMarkdown(t *testing.T) {
	md := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("HubSpot [1]"))
	}
	s := newServer(t, md)
	c := newTestClient(s)
	res, err := c.Gemini(context.Background(), AssistantParams{Prompt: "p", Format: FormatMarkdown})
	if err != nil || res.Markdown != "HubSpot [1]" || res.Request.Prompt != "p" || res.Request.Endpoint != "gemini" {
		t.Fatalf("%+v %v", res, err)
	}
	text, err := c.Markdown(context.Background(), EndpointBraveAI, AssistantParams{Prompt: "p"})
	if err != nil || text != "HubSpot [1]" || s.requests()[1].Body["format"] != "markdown" {
		t.Fatalf("%q %v", text, err)
	}
}

func TestAssistantRetriesAndTimeout(t *testing.T) {
	// A lost answer is billed: no retry on 5xx (failed answers are free).
	s := newServer(t, errReply(503, "upstream_blocked", "x", "Retry-After", "0"))
	if _, err := newTestClient(s).ChatGPT(context.Background(), ChatGPTParams{Prompt: "p"}); err == nil {
		t.Fatal("want an error")
	}
	if _, err := newTestClient(s).Markdown(context.Background(), EndpointCopilot, AssistantParams{Prompt: "p"}); err == nil {
		t.Fatal("want an error")
	}
	if n := len(s.requests()); n != 2 {
		t.Fatalf("sent %d times", n)
	}

	r := newServer(t, errReply(429, "rate_limited", "x", "Retry-After", "0"), jsonReply(200, assistantJSON))
	if _, err := newTestClient(r).AIMode(context.Background(), AssistantParams{Prompt: "p"}); err != nil || len(r.requests()) != 2 {
		t.Fatalf("429 not retried: %v %d", err, len(r.requests()))
	}

	// The per-attempt timeout is raised to the answer time.
	slow := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		jsonReply(200, assistantJSON)(w, r)
	})
	if _, err := newTestClient(slow, WithTimeout(10*time.Millisecond)).BraveAI(context.Background(), AssistantParams{Prompt: "p"}); err != nil {
		t.Fatalf("slow answer: %v", err)
	}
}

func TestSearchAIOverviewAndAIMonitors(t *testing.T) {
	s := newServer(t, jsonReply(200, `{"request":{"endpoint":"search","engine":"google","q":"x"},"results":[],"related_searches":[],
	  "ai_overview":{"text":"t","markdown":"m","sources":[{"position":1,"link":"https://a.example/","domain":"a.example","cited":true}]},
	  "meta":{"request_id":"r","credits_used":1,"cached":false}}`),
		jsonReply(201, `{"id":"m1","name":"crm","endpoint":"chatgpt","request":{"prompt":"best CRM","web_search":false,"track":["hubspot.com"]},
	  "interval_seconds":86400,"webhook_url":null,"active":true,"next_run_at":"2026-10-28T00:00:00Z","runs":0,"consecutive_failures":0,"credits_used":0,"created_at":"2026-10-28T00:00:00Z"}`))
	c := newTestClient(s)
	res, err := c.Search(context.Background(), SearchParams{Q: "x"})
	if err != nil || res.AIOverview == nil || res.AIOverview.Sources[0].Domain != "a.example" {
		t.Fatalf("%+v %v", res, err)
	}
	m, err := c.Monitors.Create(context.Background(), MonitorCreateParams{Endpoint: string(EndpointChatGPT), Prompt: "best CRM", WebSearch: Bool(false), Track: []string{"hubspot.com"}})
	if err != nil || m.Endpoint != "chatgpt" || m.Request.Prompt != "best CRM" || *m.Request.WebSearch || m.Request.Track[0] != "hubspot.com" {
		t.Fatalf("%+v %v", m, err)
	}
	b := s.requests()[1].Body
	if b["endpoint"] != "chatgpt" || b["prompt"] != "best CRM" || b["web_search"] != false || b["q"] != nil {
		t.Errorf("monitor body %+v", b)
	}

	var ev MonitorResultsEvent
	if err := json.Unmarshal([]byte(`{"event":"monitor.results","monitor_id":"m1","run_id":"r1","name":"crm","endpoint":"gemini","prompt":"best CRM",
	  "first_run":false,"run_at":"2026-10-28T00:00:00Z","credits_used":5,
	  "new_results":[{"text":"t","markdown":"m","sources":[],"new_sources":["https://a.example/"],"search_queries":[],
	    "mentions":[{"term":"Acme","mentioned":true,"cited":false,"sources":[],"text_position":0.4}]}]}`), &ev); err != nil {
		t.Fatal(err)
	}
	var a MonitorAnswer
	if err := json.Unmarshal(ev.NewResults[0], &a); err != nil || ev.Prompt != "best CRM" || ev.Q != "" || a.NewSources[0] != "https://a.example/" || a.Mentions[0].TextPosition != 0.4 {
		t.Fatalf("%+v %+v %v", ev, a, err)
	}
}
