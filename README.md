# SerpKite Go SDK

Official Go client for [SerpKite](https://serpkite.com), the Google search API built for AI agents:
clean JSON or Markdown, credits that never expire.

- Standard library only at runtime (`net/http`, `encoding/json`).
- Typed params and responses for every vertical, `context.Context` everywhere.
- Retries with exponential backoff and jitter on `429`, `5xx` and network errors (honours `Retry-After`).

## Install

```bash
go get github.com/serpkite/serpkite-go
```

```go
import serpkite "github.com/serpkite/serpkite-go"
```

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"

	serpkite "github.com/serpkite/serpkite-go"
)

func main() {
	ctx := context.Background()
	c := serpkite.NewClient() // reads SERPKITE_API_KEY

	res, err := c.Search(ctx, serpkite.SearchParams{Q: "best espresso machine", Country: "us"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.Results[0].Title, res.Meta.CreditsUsed)

	md, err := c.SearchMarkdown(ctx, serpkite.SearchParams{Q: "best espresso machine"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(md) // Markdown, ready for an LLM prompt
}
```

Get an API key at [app.serpkite.com](https://app.serpkite.com) and export it:

```bash
export SERPKITE_API_KEY=skt_live_...
go run ./examples/search "best espresso machine"
```

## Configuration

```go
c := serpkite.NewClient(
	serpkite.WithAPIKey("skt_live_..."),               // default: $SERPKITE_API_KEY
	serpkite.WithBaseURL("https://api.serpkite.com"), // default: $SERPKITE_BASE_URL or https://api.serpkite.com
	serpkite.WithHTTPClient(&http.Client{Transport: myTransport}),
	serpkite.WithMaxRetries(2),                       // retries after the first attempt
	serpkite.WithTimeout(60*time.Second),             // per attempt; the ctx deadline also applies
	serpkite.WithRetryBackoff(500*time.Millisecond),  // base of the exponential backoff
)
```

A missing key returns an `*Error` with code `missing_api_key` on the first call. The client is safe
for concurrent use; create one and share it.

Every method also accepts per-call options:

```go
var info serpkite.ResponseInfo
res, err := c.Search(ctx, serpkite.SearchParams{Q: "espresso"},
	serpkite.CaptureResponse(&info),               // billing headers of the response
	serpkite.WithHeader("X-Request-Id", "trace-123"),
)
fmt.Println(info.CreditsUsed, *info.CreditsRemaining, info.Cache, info.RequestID)
```

`ResponseInfo` carries `StatusCode`, `Header`, `RequestID`, `CreditsUsed`, `CreditsRemaining`,
`CostUSD`, `Cache` (`HIT` | `MISS`), `LatencyMs` and `TokensEstimate`.

## Methods

Every response has `Request` (the normalised request), `Results` (the vertical's primary list),
vertical-specific extras and `Meta` (`RequestID`, `CreditsUsed`, `Cached`, `LatencyMs`, …).

| Method | Endpoint | Params | Returns |
| --- | --- | --- | --- |
| `Search` | `POST /v1/search` | `SearchParams` | `*SearchResponse` (`Results`, `AnswerBox`, `KnowledgeGraph`, `PeopleAlsoAsk`, `RelatedSearches`, `TopStories`, `Places`, `Ads`) |
| `SearchMarkdown` | `POST /v1/search` | `SearchParams` | `string` |
| `Markdown` | any vertical | `Endpoint`, params | `string` |
| `Images` | `POST /v1/images` | `SearchParams` | `*ImagesResponse` |
| `Videos` | `POST /v1/videos` | `SearchParams` | `*VideosResponse` |
| `News` | `POST /v1/news` | `SearchParams` | `*NewsResponse` |
| `Maps` | `POST /v1/maps` | `SearchParams` (+ `LL`: `"@lat,lng,14z"`) | `*PlacesResponse` |
| `Places` | `POST /v1/places` | `SearchParams` | `*PlacesResponse` |
| `Place` | `POST /v1/place` | `PlaceParams` (`PlaceID` \| `CID` \| `FID`, `Country`, `Language`) | `*PlacesResponse` (one place: `WeeklyHours`, `Photos`, `Timezone`, …; empty when the id is unknown) |
| `Reviews` | `POST /v1/reviews` | `ReviewsParams` (`PlaceID` \| `CID` \| `FID`, `Sort`, `PageToken`, `Num` ≤ 10) | `*ReviewsResponse` (+ `NextPageToken`) |
| `Shopping` | `POST /v1/shopping` | `SearchParams` | `*ShoppingResponse` |
| `Scholar` | `POST /v1/scholar` | `SearchParams` | `*ScholarResponse` |
| `Patents` | `POST /v1/patents` | `SearchParams` | `*PatentsResponse` |
| `Autocomplete` | `POST /v1/autocomplete` | `SearchParams` | `*AutocompleteResponse` (`Results[i].Value`) |
| `Webpage` | `POST /v1/webpage` | `WebpageParams` (`URL`, `IncludeHTML`) | `*WebpageResponse` (`Markdown`, `Text`, `Metadata`) |
| `Rank` | `POST /v1/rank` | `RankParams` (`Q`, `Domain`, `Num`: 10\|20\|30\|50\|100) | `*RankResponse` (`Position` or nil, `Matches`, `Checked`) |
| `ChatGPT` | `POST /v1/chatgpt` | `ChatGPTParams` (`Prompt`, `WebSearch`, `Country`, `Track`, `IncludeHTML`) | `*AssistantResponse` |
| `Gemini`, `Copilot`, `AIMode`, `BraveAI` | `POST /v1/gemini`, `/v1/copilot`, `/v1/ai_mode`, `/v1/brave_ai` | `AssistantParams` (`Prompt`, `Country`, `Language`, `Track`, `IncludeHTML`) | `*AssistantResponse` (`Text`, `Markdown`, `Sources`, `SearchQueries`, `Products`, `Places`, `Mentions`) |
| `Answer` | `POST /v1/answer` | `AnswerParams` (`Q`, `Depth`: `DepthFast`\|`DepthStandard`, `NumSources` 1-5, `OutputSchema`, search options) | `*AnswerResponse` (`Answer`, `Citations`, `Results`, `Output`) |
| `AnswerStream` | `POST /v1/answer` (SSE) | `AnswerParams` | `*AnswerStream` (`Next`, `Event`, `Err`, `Collect`, `Close`) |
| `Extract` | `POST /v1/extract` | `ExtractParams` (`URLs` ≤ 20, `Format`, `Query`, `Highlights`, `Summary`, `OutputSchema`, `Prompt`, …) | `*ExtractResponse` (`Results`, `Failed`) |
| `Map` | `POST /v1/map` | `MapParams` (`URL`, `Search`, `Limit`, `Sitemap`, path filters) | `*MapResponse` (`Results[i].URL`) |
| `Crawl` | `POST /v1/crawl` | `CrawlParams` (`URL`, `Limit` ≤ 1000, `MaxDepth` ≤ 10, …) | `*TaskCreated` (`202`) |
| `GetCrawl` / `CancelCrawl` | `GET` / `DELETE /v1/crawl/{id}` | id | `*CrawlTask` / `*TaskCancelResponse` |
| `WaitForCrawl` | polls `GET /v1/crawl/{id}` | id, `WaitOptions` | `*CrawlTask` (completed, failed or canceled) |
| `Research` | `POST /v1/research` | `ResearchParams` (`Input`, `Effort`: `EffortLite`\|`EffortStandard`\|`EffortDeep`, `OutputSchema`, domains, `WebhookURL`) | `*TaskCreated` (`202`) |
| `GetResearch` / `CancelResearch` | `GET` / `DELETE /v1/research/{id}` | id | `*ResearchTask` / `*TaskCancelResponse` |
| `WaitResearch` | polls `GET /v1/research/{id}` | id, `WaitOptions` | `*ResearchTask` (completed, failed or canceled) |
| `Monitors.Create/List/Get/Update/Delete/Run` | `/v1/monitors…` | `MonitorCreateParams`, `MonitorUpdateParams` | `*Monitor`, `[]Monitor` |
| `Monitors.Runs` | `GET /v1/monitors/{id}/runs` | id, `*MonitorRunsParams` (`Limit`, `Before`) | `*MonitorRunList` (`Results`, `NextBefore`) |
| `Account` | `GET /v1/account` | none | `*Account` (`Balance`, `Plan`, `RateLimitRPS`, `Month`, …) |
| `Batches.Create` | `POST /v1/batches` | `BatchCreateParams` | `*BatchCreateResponse` |
| `Batches.Get` | `GET /v1/batches/{id}` | id | `*Batch` |
| `Batches.Wait` | polls `GET /v1/batches/{id}` | id, `WaitOptions` | `*Batch` (done or failed) |
| `Do` | any path | method, path, body, out | `*ResponseInfo` |

`SearchParams` fields: `Q` (required), `Country` (default `us`), `Language` (default `en`),
`Location`, `UULE`, `LL`, `Num` (10, or 100 for the depth bundle), `Page` (1-10), `Time`
(`hour`|`day`|`week`|`month`|`year`), `TBS`, `Device`, `Safe`, `Autocorrect`, `Format`, `Fields`,
`IncludeContent` (0-5), `Ads`, `MaxAge`, `Engine` (see
[Search engines & fallback](#search-engines--fallback)), `IncludeAnswer` (search and news, see
[AI answers](#ai-answers)). Zero values mean "API default".
`Autocorrect` defaults to true on the server, so it is a `*bool`: pass `serpkite.Bool(false)` to
turn it off.

### Examples

```go
// News from the last day in Germany
news, err := c.News(ctx, serpkite.SearchParams{Q: "EZB Zinsen", Country: "de", Language: "de", Time: "day"})

// Top 3 organic pages fetched as Markdown (+1 credit per page)
deep, err := c.Search(ctx, serpkite.SearchParams{Q: "rust async runtime comparison", IncludeContent: 3})
fmt.Println(deep.Results[0].Content)

// Search the literal query, without autocorrect
res, err := c.Search(ctx, serpkite.SearchParams{Q: "espresso", Autocorrect: serpkite.Bool(false)})

// Markdown from any vertical
md, err := c.Markdown(ctx, serpkite.EndpointNews, serpkite.SearchParams{Q: "espresso"})

// One place in full: structured hours, phones, time zone, photos
pl, err := c.Place(ctx, serpkite.PlaceParams{PlaceID: "ChIJN1t_tDeuEmsRUsoyG83frY4"})
if err == nil && len(pl.Results) > 0 && pl.Results[0].WeeklyHours != nil {
	fmt.Println(pl.Results[0].Timezone, pl.Results[0].WeeklyHours.Monday) // [{09:00 17:00 false}]
}

// Reviews, paged (the first 10 of a place)
p := serpkite.ReviewsParams{PlaceID: "ChIJN1t_tDeuEmsRUsoyG83frY4", Num: 5}
for {
	page, err := c.Reviews(ctx, p)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range page.Results {
		fmt.Println(r.Rating, r.Snippet)
	}
	if page.NextPageToken == "" {
		break
	}
	p.PageToken = page.NextPageToken
}

// Any URL as Markdown
doc, err := c.Webpage(ctx, serpkite.WebpageParams{URL: "https://example.com/blog/post"})
fmt.Println(doc.Metadata.Title, doc.Markdown)

// Where does a domain rank?
rank, err := c.Rank(ctx, serpkite.RankParams{Q: "espresso machine", Domain: "example.com"})
if rank.Position != nil {
	fmt.Println("position", *rank.Position)
}

// Balance
acct, err := c.Account(ctx)
fmt.Println(acct.Balance, acct.Month.Credits)

// Cached result up to one hour old (half price on a hit)
res, err = c.Search(ctx, serpkite.SearchParams{Q: "espresso", MaxAge: 3600})
```

With `Fields` the response contains only the requested keys; the other struct fields stay zero.
`Format: "compact"` is best read with `Do` into a `map[string]any`.

## Search controls

Domain filters and date ranges work on search, news, images and videos; `BoostDomains` on search
and news; `Highlights` on search with `IncludeContent`. None of them costs extra credits.

```go
res, err := c.Search(ctx, serpkite.SearchParams{
	Q:              "connection pooling",
	IncludeDomains: []string{"postgresql.org", "github.com/pgbouncer", ".edu"}, // host, path prefix or TLD (≤ 20)
	ExcludeDomains: []string{"pinterest.com"},
	BoostDomains:   []string{"postgresql.org"}, // to the top, keeping the rest
	StartDate:      "2026-01-01",               // YYYY-MM-DD, EndDate too
	IncludeContent: 3,
	Highlights:     true, // 3 query-ranked passages per page instead of the whole page
})
for _, r := range res.Results {
	fmt.Println(r.Position, r.PublishedAt, len(r.Highlights))
}
```

## Search engines & fallback

By default every request is answered by Google only (a nil `Engine`); SerpKite already fails over
across its own proxy pools. Opt in to other providers with `Engine`:

```go
// Fall back to other enabled providers when Google is blocked or times out
res, err := c.Search(ctx, serpkite.SearchParams{Q: "best espresso machine", Engine: serpkite.Engines(serpkite.EngineAuto)})
fmt.Println(res.Meta.Engine) // "google", or e.g. "brave" if Google was unavailable
for _, step := range res.Meta.Route { // nil on cache hits
	fmt.Println(step.Provider, step.Outcome, step.Ms) // google blocked 812 / brave ok 431
}

// Only these providers, in this order
news, err := c.News(ctx, serpkite.SearchParams{Q: "espresso", Engine: serpkite.Engines(serpkite.ProviderGoogle, serpkite.ProviderBrave)})
```

- `Engine` is a `[]string`: one name (`ProviderGoogle`, `EngineAuto`, `EngineConsensus`,
  `ProviderBrave`, `ProviderBing`, `ProviderYahoo`, `ProviderDuckDuckGo`, `ProviderMojeek`, `ProviderWikipedia`,
  `ProviderYandex`) is
  sent as a JSON string, several as an array. `EngineAuto` and `EngineConsensus` can't be combined
  with other names; unknown names, or a provider that doesn't serve the endpoint, return a 400
  `invalid_request`. `Engine.String()` gives the query-string form (`google,brave`).
- `EngineConsensus` (`Search` only) asks several independent indexes in parallel, merges the
  results by URL and ranks them by agreement: each `OrganicResult.Sources` lists the providers
  that returned it, `Meta.Engine` is `"consensus"`, and it costs the sum of one page per provider
  that returned results.
- `Meta.Engine` names the provider that answered; `Meta.Route` (`[]RouteStep`) lists each attempt
  and its `Outcome`. `Request.Engine` echoes what you asked for.
- Credits (`Meta.CreditsUsed`, `X-Credits-Used`) follow the answering provider's price.

## AI answers

```go
// A cited answer grounded on live Google results
res, err := c.Answer(ctx, serpkite.AnswerParams{Q: "what is retrieval-augmented generation", Depth: serpkite.DepthFast})
if err != nil {
	log.Fatal(err)
}
fmt.Println(res.Text()) // Markdown with [1] markers
for _, cite := range res.Citations {
	fmt.Println(cite.Index, cite.Title, cite.Link)
}

// Stream it: one "results" event, "delta" events with the text, then "done"
s, err := c.AnswerStream(ctx, serpkite.AnswerParams{Q: "what is retrieval-augmented generation"})
if err != nil {
	log.Fatal(err)
}
defer s.Close()
for s.Next() {
	if ev := s.Event(); ev.Delta != nil {
		fmt.Print(ev.Delta.Text)
	}
}
if err := s.Err(); err != nil { // e.g. code "ai_unavailable"
	log.Fatal(err)
}

// Structured output: the answer as JSON matching a schema (not with streaming)
facts, err := c.Answer(ctx, serpkite.AnswerParams{
	Q:            "tallest building in the world",
	OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
})
var building struct{ Name string }
_, err = facts.DecodeOutput(&building)

// A cited answer on top of a normal search or news response
news, err := c.News(ctx, serpkite.SearchParams{Q: "ECB rate decision", Time: "day", IncludeAnswer: true})
if news.Answer != nil {
	fmt.Println(news.Answer.Text)
} else {
	fmt.Println(news.Meta.Warnings) // the model failed: refunded
}
```

- `DepthFast` answers from the result page; `DepthStandard` (default) also reads the top
  `NumSources` pages. `Meta.Credits` splits the cost into the search and the model part and
  `Meta.LLM` names the model. Prices: [serpkite.com/pricing](https://serpkite.com/pricing).
- A model failure is an `*Error` with code `ai_unavailable` (503) and the model part is refunded.
  In a stream it arrives as an `error` event (`s.Err()`). `Close` before `done` stops generation
  and the model part is not charged. `WithTimeout` bounds the wait for the response headers only
  (at least 100 s, since the search runs first); bound a stream with `ctx`. `s.Collect()` reads
  the rest and returns the `done` event. `Answer` is billed even when its response is lost, so it
  is retried only on 429.
- `Extract` takes `Summary` (an AI summary per page) and `OutputSchema` (+ optional `Prompt`; each
  page's data as JSON in `Output`, decode with `DecodeOutput`). A page whose model step failed
  carries `SummaryError` / `OutputError` instead. The default `Timeout` becomes 75 seconds with
  either.

## AI assistants

Ask ChatGPT, Gemini, Copilot (Bing Copilot Search), Google AI Mode or Brave's AI answer a prompt as
a logged-out user and get the answer with the sources it cited, the searches it ran and the
products and places it showed. `Track` lists brands and domains to look for; each comes back in
`Mentions`.

```go
res, err := c.ChatGPT(ctx, serpkite.ChatGPTParams{
	Prompt:  "What are the best CRMs for a 10-person startup?",
	Country: "us", // ask from an exit in this country (prices and shops follow it)
	Track:   []string{"hubspot.com", "Pipedrive"},
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(res.Model, res.Markdown) // citations as [n](link), matching Sources[].Position
for _, s := range res.Sources {
	fmt.Println(s.Position, s.Cited, s.Domain)
}
for _, m := range res.Mentions {
	// Mentioned: the text names it; Cited: a cited source matches it; TextPosition 0-1 (-1: not named)
	fmt.Println(m.Term, m.Mentioned, m.Cited, m.Sources, m.TextPosition)
}

md, err := c.Markdown(ctx, serpkite.EndpointAIMode, serpkite.AssistantParams{Prompt: "how to remove a red wine stain from wool"})
_, err = c.Gemini(ctx, serpkite.AssistantParams{Prompt: "Wie funktionieren Wärmepumpen?", Country: "de", Language: "de"})

// Google's AI Overview on a normal search, when the page shows one
serp, err := c.Search(ctx, serpkite.SearchParams{Q: "how do heat pumps work"})
if serp.AIOverview != nil {
	fmt.Println(serp.AIOverview.Text, serp.AIOverview.Sources)
}
```

- Prices per answer: ChatGPT 4, Gemini 3, Copilot 3, AI Mode 2, Brave AI 2 credits (the same in
  batches; `MaxAge` cache hits half). Failed, refused and empty answers are free.
- Answers take 10-90 s: each attempt waits at least 120 s (raised from `WithTimeout`; bound it
  with `ctx`) and is retried only on 429 (a lost response is still billed). `Markdown` with an
  `Endpoint*` assistant constant follows the same rules.
- `WebSearch` (default true; `serpkite.Bool(false)` lets ChatGPT decide) is ChatGPT only;
  `Language` isn't taken by ChatGPT.
- Run many prompts through `Batches.Create` with `Endpoint: serpkite.EndpointChatGPT`, or track an
  answer over time with a monitor (see [Monitors](#monitors)).

## Research

```go
task, err := c.Research(ctx, serpkite.ResearchParams{
	Input:  "Compare managed Postgres vector search options in 2026",
	Effort: serpkite.EffortStandard, // EffortLite | EffortStandard (default) | EffortDeep
})
if err != nil {
	log.Fatal(err)
}
done, err := c.WaitResearch(ctx, task.ID) // completed, failed or canceled (default timeout 15 minutes)
if err == nil && done.Status == serpkite.TaskCompleted && done.Result != nil {
	fmt.Println(done.Result.Report) // Markdown with [n] citations
	for _, src := range done.Result.Sources {
		fmt.Println(src.Index, src.Link, src.Query)
	}
}
```

- Each `Effort` has a flat price ([serpkite.com/pricing](https://serpkite.com/pricing)), reserved
  up front and charged only when the report is written; failed research is refunded. `Research`
  is only retried on 429.
- `GetResearch` shows `Progress` (`Round`, `Queries`, `Sources`, `Step`) while it runs.
  `CancelResearch` refunds a queued task; a running one stops at its next round and is charged
  for the searches already run.
- `OutputSchema` also returns the report as JSON in `Result.Output`. With `WebhookURL` the task
  ends with a signed `research.completed` delivery (`serpkite.TaskCompletedEvent`, `Kind`
  `"research"`; decode `Result` into a `serpkite.ResearchResult`).

## Map and extract

```go
// The URLs of a site (1 credit): sitemaps + start page, canonicalised and deduplicated
site, err := c.Map(ctx, serpkite.MapParams{URL: "https://docs.example.com/", Search: "install", IncludePaths: []string{"^/guides/"}})

// Up to 20 URLs (HTML or PDF) as Markdown in one call: 1 credit per URL that came back
pages, err := c.Extract(ctx, serpkite.ExtractParams{URLs: []string{site.Results[0].URL}, Query: "install", Highlights: 3})
for _, f := range pages.Failed { // not charged
	fmt.Println("failed", f.URL, f.Error.Code)
}
```

## Crawl

```go
task, err := c.Crawl(ctx, serpkite.CrawlParams{
	URL:          "https://docs.example.com/",
	Limit:        200,              // up to 1000 pages
	MaxDepth:     serpkite.Int(3), // up to 10; Int(0) reads only URL
	IncludePaths: []string{"^/guides/"},
	Sitemap:      "include",        // include (default) | only | skip
	Query:        "authentication", // read the most relevant pages first
})
if err != nil {
	log.Fatal(err)
}
done, err := c.WaitForCrawl(ctx, task.ID) // completed, failed or canceled
if err == nil && done.Result != nil {
	for _, p := range done.Result.Pages {
		fmt.Println(p.URL, len(p.Markdown))
	}
	fmt.Println(done.Result.Stats.Stopped) // done | limit | time_limit | size_limit | too_many_failures | canceled
}
```

- `Crawl` reserves `Limit` credits and charges 1 per page read (0.5 from cache); the rest is
  refunded. It is only retried on 429, so a lost response never starts a second crawl.
- `WaitForCrawl(ctx, id, serpkite.WaitOptions{…})` polls every 2 s (×1.5, up to 15 s; default
  timeout 35 minutes). A failed or canceled crawl is returned without an error; on timeout it
  returns the last poll and an `*Error` with code `timeout`.
- `CancelCrawl` refunds a queued crawl; a running one stops at its next checkpoint (`canceling`).
- With `WebhookURL` the crawl ends with a signed `crawl.completed` delivery
  (`serpkite.TaskCompletedEvent`; decode `Result` with `DecodeResult`). A result over 4 MB arrives
  with `ResultOmitted` set: fetch it with `GetCrawl`.

## Monitors

```go
m, err := c.Monitors.Create(ctx, serpkite.MonitorCreateParams{
	Q:          "ai agents",
	Endpoint:   "news",
	Interval:   serpkite.IntervalHourly, // or IntervalSeconds (3600-2592000)
	WebhookURL: "https://example.com/hooks/serpkite", // optional
})
_, err = c.Monitors.Run(ctx, m.ID) // due within ~30 s
_, err = c.Monitors.Update(ctx, m.ID, serpkite.MonitorUpdateParams{Active: serpkite.Bool(false)})

// Run history, newest first (new results kept 24 h)
p := &serpkite.MonitorRunsParams{Limit: 20}
for {
	page, err := c.Monitors.Runs(ctx, m.ID, p)
	if err != nil {
		log.Fatal(err)
	}
	for _, run := range page.Results {
		fmt.Println(run.Status, run.NewResults) // run.Results: []json.RawMessage
	}
	if page.NextBefore == nil {
		break
	}
	p.Before = *page.NextBefore
}

// Watch a page for content changes (1 credit per check); Metadata is echoed in webhooks
w, err := c.Monitors.Create(ctx, serpkite.MonitorCreateParams{
	Endpoint: string(serpkite.EndpointWebpage),
	URL:      "https://example.com/pricing",
	Metadata: map[string]any{"customer": "acme"},
})
_, err = c.Monitors.Update(ctx, w.ID, serpkite.MonitorUpdateParams{Metadata: serpkite.ClearMetadata})
err = c.Monitors.Delete(ctx, w.ID)

// Ask an AI assistant every day and see who it cites (each run costs the endpoint's price)
_, err = c.Monitors.Create(ctx, serpkite.MonitorCreateParams{
	Endpoint: string(serpkite.EndpointChatGPT),
	Prompt:   "What are the best CRMs for a 10-person startup?",
	Track:    []string{"hubspot.com", "Pipedrive"},
})
```

An AI assistant monitor (`chatgpt`, `gemini`, `copilot`, `ai_mode`, `brave_ai`) reports every
answer as one `serpkite.MonitorAnswer` (`Text`, `Markdown`, `Sources`, `NewSources` (links not seen
in earlier runs), `SearchQueries`, `Products` and, with `Track`, `Mentions`); unmarshal it from
`run.Results[0]` or the webhook's `NewResults[0]`. Its webhook carries `Prompt` instead of `Q`.

Each search run costs what its search costs (1 credit per 10 results, `num` 100 is 7; empty and failed runs are free) and reports only results
it hasn't seen before: as a signed `monitor.results` webhook (`serpkite.MonitorResultsEvent`; dedupe
retries on `RunID`) and in `Runs`. A monitor pauses itself after 10 failed runs in a row; `Update`
with `Active: serpkite.Bool(true)` resumes it, and `WebhookURL: serpkite.String("")` removes the
webhook.

## Batches

Batch jobs cost half price. Submit 1-100 requests for one endpoint; each request becomes a job.

```go
job, err := c.Batches.Create(ctx, serpkite.BatchCreateParams{
	Endpoint:   serpkite.EndpointSearch, // or EndpointNews, EndpointWebpage, …
	Requests:   []any{serpkite.SearchParams{Q: "a"}, serpkite.SearchParams{Q: "b"}},
	WebhookURL: "https://example.com/hooks/serpkite", // optional
})
if err != nil {
	log.Fatal(err)
}
for i, e := range job.Batches {
	if e.Err != nil {
		fmt.Println("request", i, "rejected:", e.Err.Code, e.Err.Message)
	}
}

done, err := c.Batches.Wait(ctx, job.Batches[0].Batch.ID) // polls until done/failed
if err != nil {
	log.Fatal(err)
}
var res serpkite.SearchResponse
if ok, err := done.DecodeResult(&res); ok && err == nil {
	fmt.Println(res.Results[0].Title)
}
```

- `Create` returns one `BatchEntry` per request, in order; exactly one of `Batch` (queued) and
  `Err` (rejected) is set. `job.Queued()` returns the accepted jobs.
- `Create(ctx, p, serpkite.WithIdempotencyKey(id))` makes the call safe to retry: the server replays
  the first response for the same key and body for 24 hours, and the client then also retries 5xx
  and network errors.
- Webhook deliveries are always signed; check one with
  `serpkite.VerifyWebhook(secret, rawBody, r.Header, time.Now())`.
- `Wait(ctx, id, serpkite.WaitOptions{Timeout, PollInterval, MaxPollInterval})` polls with a growing
  interval (1 s, ×1.5, up to 10 s; default timeout 10 minutes, and the context deadline applies).
  A failed job is returned without an error: check `Status` and `Error`. On timeout it returns an
  `*Error` with code `timeout`.
- `Batch.Result` is raw JSON (kept 24 h). Decode it into the endpoint's response type with
  `DecodeResult`. For `format: "markdown"` requests the result is `{"markdown": "...", "meta": {...}}`.
- Webhooks are signed with `X-SerpKite-Signature` (HMAC-SHA256 of `<X-SerpKite-Timestamp>.<body>`
  with your webhook secret).

## Errors

Every failure is an `*serpkite.Error` (context cancellation returns the context error):

```go
res, err := c.Search(ctx, serpkite.SearchParams{Q: "espresso"})
var apiErr *serpkite.Error
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Status, apiErr.Code, apiErr.Message, apiErr.RequestID)
}
```

| `Code` | `Status` | Meaning |
| --- | --- | --- |
| `invalid_request` | 400 | Bad or unknown parameter |
| `unauthorized` | 401 | Missing or invalid API key |
| `insufficient_credits` | 402 | Balance too low |
| `spend_cap_reached`, `key_limit_reached` | 403 | Account spend cap or per-key monthly limit hit |
| `forbidden` | 403 | Not allowed |
| `not_found` | 404 | Unknown batch id |
| `rate_limited` | 429 | Too many requests (retried automatically) |
| `daily_limit_reached` | 429 | The free daily limit is used up until 00:00 UTC; buying any pack removes it (never retried, `serpkite.CodeDailyLimit`) |
| `internal` | 500 | Server error (retried) |
| `upstream_error`, `upstream_blocked` | 503 | Google could not be fetched (not billed, retried) |
| `upstream_timeout` | 503 | The search took too long (not billed, retried) |
| `ai_unavailable` | 503 | The AI model failed or AI is off (model part refunded; retried, honours `Retry-After`) |
| `unavailable` | 503 | Temporarily unavailable (retried) |
| `connection_error`, `timeout` | 0 | No response received |
| `missing_api_key` | 0 | No key passed and `SERPKITE_API_KEY` unset |

`apiErr.Retryable()` reports whether a failure is worth retrying later. Failed, empty and blocked
searches are refunded, so they never cost credits.

## Retries

Requests are retried up to `WithMaxRetries` times (default 2) on `429` (except
`daily_limit_reached`), `5xx`, network errors and timeouts, with exponential backoff (`500 ms · 2^attempt`, capped at 8 s) and jitter. A `Retry-After`
header (seconds or HTTP date, capped at 60 s) takes precedence. Other `4xx` errors are never
retried. `Batches.Create`, `Crawl`, `Research`, `Extract`, `Answer`, the AI assistant methods,
`Monitors.Create` and `Monitors.Run` retry only on `429`, so a lost response can never queue (and bill) the same work twice.
`AnswerStream` retries only until the stream starts.

## Types

The request and response structs in `types*.go` are **hand-written** from the OpenAPI contract
(`backend/api/serp-api.yaml`) rather than generated. `oapi-codegen` produces pointers for every
optional field (`*string` for `Country`), `Id`/`ImageUrl`-style names and union wrapper types for
the batch entries, which makes for an awkward API. Hand-written structs use plain values with
`omitempty`, Go initialisms (`URL`, `ID`, `ImageURL`) and `*bool` only where the server default is
true. `contract_test.go` keeps them honest: it loads the contract and fails when a struct is
missing a property, has one the contract does not define, or marks a required key `omitempty`.

When the contract changes, edit the `types*.go` files and run:

```bash
go test ./...   # contract_test.go reports every drifted field
```

## Development

```bash
go vet ./...
go test ./...
gofmt -l .
```

## License

MIT
