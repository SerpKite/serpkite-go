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
| `Reviews` | `POST /v1/reviews` | `ReviewsParams` (`PlaceID` \| `CID` \| `FID`, `Sort`, `PageToken`, `Num` ≤ 50) | `*ReviewsResponse` (+ `NextPageToken`) |
| `Shopping` | `POST /v1/shopping` | `SearchParams` | `*ShoppingResponse` |
| `Scholar` | `POST /v1/scholar` | `SearchParams` | `*ScholarResponse` |
| `Patents` | `POST /v1/patents` | `SearchParams` | `*PatentsResponse` |
| `Autocomplete` | `POST /v1/autocomplete` | `SearchParams` | `*AutocompleteResponse` (`Results[i].Value`) |
| `Webpage` | `POST /v1/webpage` | `WebpageParams` (`URL`, `IncludeHTML`) | `*WebpageResponse` (`Markdown`, `Text`, `Metadata`) |
| `Rank` | `POST /v1/rank` | `RankParams` (`Q`, `Domain`, `Num`: 10\|20\|30\|50\|100) | `*RankResponse` (`Position` or nil, `Matches`, `Checked`) |
| `Account` | `GET /v1/account` | none | `*Account` (`Balance`, `Plan`, `RateLimitRPS`, `Month`, …) |
| `Batches.Create` | `POST /v1/batches` | `BatchCreateParams` | `*BatchCreateResponse` |
| `Batches.Get` | `GET /v1/batches/{id}` | id | `*Batch` |
| `Batches.Wait` | polls `GET /v1/batches/{id}` | id, `WaitOptions` | `*Batch` (done or failed) |
| `Do` | any path | method, path, body, out | `*ResponseInfo` |

`SearchParams` fields: `Q` (required), `Country` (default `us`), `Language` (default `en`),
`Location`, `UULE`, `LL`, `Num` (10, or 100 for the depth bundle), `Page` (1-10), `Time`
(`hour`|`day`|`week`|`month`|`year`), `TBS`, `Device`, `Safe`, `Autocorrect`, `Format`, `Fields`,
`IncludeContent` (0-5), `Ads`, `MaxAge`, `Engine` (see
[Search engines & fallback](#search-engines--fallback)). Zero values mean "API default".
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

// Reviews, paged
p := serpkite.ReviewsParams{PlaceID: "ChIJN1t_tDeuEmsRUsoyG83frY4", Sort: "newest"}
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
  `ProviderBrave`, `ProviderBing`, `ProviderYahoo`, `ProviderDuckDuckGo`, `ProviderMojeek`, `ProviderWikipedia`) is
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
| `internal` | 500 | Server error (retried) |
| `upstream_error`, `upstream_blocked` | 503 | Google could not be fetched (not billed, retried) |
| `upstream_timeout` | 503 | The search took too long (not billed, retried) |
| `unavailable` | 503 | Temporarily unavailable (retried) |
| `connection_error`, `timeout` | 0 | No response received |
| `missing_api_key` | 0 | No key passed and `SERPKITE_API_KEY` unset |

`apiErr.Retryable()` reports whether a failure is worth retrying later. Failed, empty and blocked
searches are refunded, so they never cost credits.

## Retries

Requests are retried up to `WithMaxRetries` times (default 2) on `429`, `5xx`, network errors and
timeouts, with exponential backoff (`500 ms · 2^attempt`, capped at 8 s) and jitter. A `Retry-After`
header (seconds or HTTP date, capped at 60 s) takes precedence. Other `4xx` errors are never
retried. `Batches.Create` retries only on `429`, so a lost response can never queue (and bill) the
same requests twice.

## Types

The request and response structs in `types.go` are **hand-written** from the OpenAPI contract
(`backend/api/serp-api.yaml`) rather than generated. `oapi-codegen` produces pointers for every
optional field (`*string` for `Country`), `Id`/`ImageUrl`-style names and union wrapper types for
the batch entries, which makes for an awkward API. Hand-written structs use plain values with
`omitempty`, Go initialisms (`URL`, `ID`, `ImageURL`) and `*bool` only where the server default is
true. `contract_test.go` keeps them honest: it loads the contract and fails when a struct is
missing a property, has one the contract does not define, or marks a required key `omitempty`.

When the contract changes, edit `types.go` and run:

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
