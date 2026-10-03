package serpkite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The types in this file mirror backend/api/serp-api.yaml (the OpenAPI
// contract). They are written by hand for a clean API: optional request fields
// are plain values with omitempty (the zero value means "use the API default"),
// except booleans whose default is true, which are *bool (use [Bool]).
// contract_test.go checks every JSON key against the contract.

// Endpoint names a vertical. Used by [Client.Markdown] and batches.
type Endpoint string

// Verticals accepted by [Client.Markdown] and [BatchCreateParams].
const (
	EndpointSearch       Endpoint = "search"
	EndpointImages       Endpoint = "images"
	EndpointVideos       Endpoint = "videos"
	EndpointNews         Endpoint = "news"
	EndpointMaps         Endpoint = "maps"
	EndpointPlaces       Endpoint = "places"
	EndpointReviews      Endpoint = "reviews"
	EndpointShopping     Endpoint = "shopping"
	EndpointScholar      Endpoint = "scholar"
	EndpointPatents      Endpoint = "patents"
	EndpointAutocomplete Endpoint = "autocomplete"
	EndpointWebpage      Endpoint = "webpage"
)

// Values for the Format field.
const (
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
	FormatCompact  = "compact"
)

// Bool returns a pointer to v, for the *bool fields of the params structs.
func Bool(v bool) *bool { return &v }

// ── Search engines ───────────────────────────────────────────────────────

// Search providers for [Engine]. [Meta].Engine names the one that answered.
const (
	ProviderGoogle     = "google"
	ProviderBrave      = "brave"
	ProviderBing       = "bing"
	ProviderYahoo      = "yahoo"
	ProviderDuckDuckGo = "duckduckgo"
	ProviderMojeek     = "mojeek"
	ProviderWikipedia  = "wikipedia"
)

// EngineAuto lets other enabled providers answer, in route order, when Google
// is blocked or times out. It can't be combined with other names.
const EngineAuto = "auto"

// EngineConsensus (Search only) asks several independent indexes in
// parallel, merges the results by URL and ranks them by how many providers
// returned each one ([OrganicResult].Sources). [Meta].Engine is
// "consensus"; it costs the sum of one page per provider that returned
// results. It can't be combined with other names.
const EngineConsensus = "consensus"

// Engine is the engine parameter: which search providers may answer. Nil (the
// zero value) is the API default, Google only. Use [Engines]:
//
//	serpkite.Engines(serpkite.EngineAuto)                             // fall back when Google is unavailable
//	serpkite.Engines(serpkite.ProviderBrave)                          // Brave only
//	serpkite.Engines(serpkite.ProviderGoogle, serpkite.ProviderBrave) // only these, in order
//
// One name marshals to a JSON string, several to an array. Unknown names, or a
// provider that doesn't serve the endpoint, are a 400. Credits follow the
// price of the provider that answered.
type Engine []string

// Engines builds an [Engine] from provider names (or [EngineAuto]).
func Engines(names ...string) Engine { return Engine(names) }

// String is the query-string form: names joined by commas.
func (e Engine) String() string { return strings.Join(e, ",") }

// MarshalJSON writes one name as a string and several as an array.
func (e Engine) MarshalJSON() ([]byte, error) {
	switch len(e) {
	case 0:
		return []byte("null"), nil
	case 1:
		return json.Marshal(e[0])
	default:
		return json.Marshal([]string(e))
	}
}

// UnmarshalJSON accepts a string ("google", "auto", "google,brave") or an
// array of strings.
func (e *Engine) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*e = nil
		return nil
	}
	if len(b) > 0 && b[0] == '[' {
		var list []string
		if err := json.Unmarshal(b, &list); err != nil {
			return fmt.Errorf("serpkite: engine: %w", err)
		}
		*e = list
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return fmt.Errorf("serpkite: engine: %w", err)
	}
	*e = strings.Split(one, ",")
	return nil
}

// RouteStep is one provider attempt behind an answer ([Meta].Route).
type RouteStep struct {
	Provider string `json:"provider"`
	// Outcome is ok, partial, empty, soft_empty, blocked, timeout, error,
	// unsupported, bad_param, bad_target or canceled.
	Outcome string `json:"outcome"`
	// Ms is the duration of the attempt in milliseconds.
	Ms int `json:"ms"`
}

// ── Requests ─────────────────────────────────────────────────────────────

// SearchParams are the parameters of Search, Images, Videos, News, Maps,
// Places, Shopping, Scholar, Patents and Autocomplete.
type SearchParams struct {
	// Q is the query (required).
	Q string `json:"q"`
	// Country is an ISO 3166-1 alpha-2 code. API default "us".
	Country string `json:"country,omitempty"`
	// Language is the interface language (e.g. en, de, pt-br). API default "en".
	Language string `json:"language,omitempty"`
	// Location is free text, e.g. "Austin, Texas, United States".
	Location string `json:"location,omitempty"`
	// UULE is a pre-encoded Google location (overrides Location).
	UULE string `json:"uule,omitempty"`
	// LL is a Maps viewport "@lat,lng,14z" (maps only).
	LL string `json:"ll,omitempty"`
	// Num is 10 (default), or 100 for the depth bundle (7 credits).
	Num int `json:"num,omitempty"`
	// Page is 1-10.
	Page int `json:"page,omitempty"`
	// Time limits results to the last hour, day, week, month or year.
	Time string `json:"time,omitempty"`
	// TBS is a raw Google tbs filter (e.g. "qdr:d"); overrides Time.
	TBS string `json:"tbs,omitempty"`
	// Device is "desktop" (default) or "mobile".
	Device string `json:"device,omitempty"`
	// Safe is "active" or "off" (default).
	Safe string `json:"safe,omitempty"`
	// Autocorrect defaults to true; set Bool(false) to search the literal query.
	Autocorrect *bool `json:"autocorrect,omitempty"`
	// Format is "json" (default), "markdown" or "compact". Prefer
	// [Client.SearchMarkdown] / [Client.Markdown] for Markdown.
	Format string `json:"format,omitempty"`
	// Fields is a comma-separated projection, e.g. "results.title,results.link,answer_box".
	Fields string `json:"fields,omitempty"`
	// IncludeContent fetches the top N (0-5) organic pages as Markdown (+1 credit each).
	IncludeContent int `json:"include_content,omitempty"`
	// Ads includes sponsored results.
	Ads bool `json:"ads,omitempty"`
	// MaxAge accepts a cached result up to this many seconds old (50% of credits on a hit).
	MaxAge int `json:"max_age,omitempty"`
	// Engine names the search providers that may answer; nil is Google only.
	// See [Engine] and [Engines].
	Engine Engine `json:"engine,omitempty"`
}

// ReviewsParams are the parameters of [Client.Reviews]. Set one of PlaceID, CID or FID.
type ReviewsParams struct {
	PlaceID  string `json:"place_id,omitempty"`
	CID      string `json:"cid,omitempty"`
	FID      string `json:"fid,omitempty"`
	Country  string `json:"country,omitempty"`
	Language string `json:"language,omitempty"`
	// Sort is most_relevant (default), newest, highest_rating or lowest_rating.
	Sort string `json:"sort,omitempty"`
	// PageToken is NextPageToken from the previous page.
	PageToken string `json:"page_token,omitempty"`
	// Num is up to 50 (default 10); 1 credit per 10 reviews.
	Num    int    `json:"num,omitempty"`
	Format string `json:"format,omitempty"`
	Fields string `json:"fields,omitempty"`
	MaxAge int    `json:"max_age,omitempty"`
}

// WebpageParams are the parameters of [Client.Webpage].
type WebpageParams struct {
	// URL to fetch (required).
	URL string `json:"url"`
	// Format is "json" (default) or "markdown". The JSON response already
	// carries the page Markdown, so this is rarely needed.
	Format string `json:"format,omitempty"`
	// IncludeHTML also returns the raw HTML.
	IncludeHTML bool `json:"include_html,omitempty"`
	MaxAge      int  `json:"max_age,omitempty"`
}

// RankParams are the parameters of [Client.Rank].
type RankParams struct {
	// Q is the keyword (required).
	Q string `json:"q"`
	// Domain to find, e.g. "example.com" (subdomains match; required).
	Domain string `json:"domain"`
	// Num is 10, 20, 30, 50 or 100 (default).
	Num      int    `json:"num,omitempty"`
	Country  string `json:"country,omitempty"`
	Language string `json:"language,omitempty"`
	Location string `json:"location,omitempty"`
	Device   string `json:"device,omitempty"`
	MaxAge   int    `json:"max_age,omitempty"`
}

// ── Shared ───────────────────────────────────────────────────────────────

// Meta is present on every response.
type Meta struct {
	RequestID   string  `json:"request_id"`
	CreditsUsed float64 `json:"credits_used"`
	// Engine is the provider that produced the result (google, brave, …).
	Engine string `json:"engine,omitempty"`
	// Route lists the provider attempts in order (fresh results only; nil on cache hits).
	Route        []RouteStep `json:"route,omitempty"`
	Cached       bool        `json:"cached"`
	CachedAt     *time.Time  `json:"cached_at,omitempty"`
	ResolvedURLs bool        `json:"resolved_urls,omitempty"`
	// ParseQuality is ok, partial or empty.
	ParseQuality string `json:"parse_quality,omitempty"`
	LatencyMs    int    `json:"latency_ms,omitempty"`
}

// RequestEcho is the normalised request that produced a response (defaults filled in).
type RequestEcho struct {
	Endpoint string `json:"endpoint"`
	// Engine is the engine policy as asked (google, auto, one provider or a list).
	Engine         Engine `json:"engine"`
	Q              string `json:"q,omitempty"`
	URL            string `json:"url,omitempty"`
	Country        string `json:"country,omitempty"`
	Language       string `json:"language,omitempty"`
	Location       string `json:"location,omitempty"`
	Num            int    `json:"num,omitempty"`
	Page           int    `json:"page,omitempty"`
	Device         string `json:"device,omitempty"`
	Autocorrect    *bool  `json:"autocorrect,omitempty"`
	TBS            string `json:"tbs,omitempty"`
	Safe           string `json:"safe,omitempty"`
	PlaceID        string `json:"place_id,omitempty"`
	CID            string `json:"cid,omitempty"`
	FID            string `json:"fid,omitempty"`
	Sort           string `json:"sort,omitempty"`
	IncludeContent int    `json:"include_content,omitempty"`
	Format         string `json:"format,omitempty"`
}

// ── Search ───────────────────────────────────────────────────────────────

// Sitelink is a sub-link under an organic result.
type Sitelink struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet,omitempty"`
}

// OrganicResult is one web result (also used for ads).
type OrganicResult struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	// Link is the resolved destination URL (never a Google redirect).
	Link string `json:"link"`
	// Domain is the canonical host without www.
	Domain        string            `json:"domain"`
	DisplayedLink string            `json:"displayed_link,omitempty"`
	Snippet       string            `json:"snippet,omitempty"`
	Date          string            `json:"date,omitempty"`
	Sitelinks     []Sitelink        `json:"sitelinks,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	Rating        float64           `json:"rating,omitempty"`
	RatingCount   int               `json:"rating_count,omitempty"`
	// Content is the page Markdown when IncludeContent covered this result.
	Content string `json:"content,omitempty"`
	// Sources lists the providers that returned this result (EngineConsensus only).
	Sources []string `json:"sources,omitempty"`
}

// AnswerBox is the featured answer.
type AnswerBox struct {
	Title              string   `json:"title,omitempty"`
	Answer             string   `json:"answer,omitempty"`
	Snippet            string   `json:"snippet,omitempty"`
	SnippetHighlighted []string `json:"snippet_highlighted,omitempty"`
	Link               string   `json:"link,omitempty"`
}

// KnowledgeGraph is the entity panel.
type KnowledgeGraph struct {
	Title             string            `json:"title,omitempty"`
	Type              string            `json:"type,omitempty"`
	Website           string            `json:"website,omitempty"`
	ImageURL          string            `json:"image_url,omitempty"`
	Description       string            `json:"description,omitempty"`
	DescriptionSource string            `json:"description_source,omitempty"`
	DescriptionLink   string            `json:"description_link,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

// PeopleAlsoAsk is one related question.
type PeopleAlsoAsk struct {
	Question string `json:"question"`
	Snippet  string `json:"snippet,omitempty"`
	Title    string `json:"title,omitempty"`
	Link     string `json:"link,omitempty"`
}

// RelatedSearch is a related query.
type RelatedSearch struct {
	Query string `json:"query"`
}

// SearchResponse is returned by [Client.Search].
type SearchResponse struct {
	Request         RequestEcho     `json:"request"`
	Results         []OrganicResult `json:"results"`
	AnswerBox       *AnswerBox      `json:"answer_box,omitempty"`
	KnowledgeGraph  *KnowledgeGraph `json:"knowledge_graph,omitempty"`
	Ads             []OrganicResult `json:"ads,omitempty"`
	PeopleAlsoAsk   []PeopleAlsoAsk `json:"people_also_ask,omitempty"`
	RelatedSearches []RelatedSearch `json:"related_searches"`
	TopStories      []NewsResult    `json:"top_stories,omitempty"`
	Places          []PlaceResult   `json:"places,omitempty"`
	Meta            Meta            `json:"meta"`
}

// ── Images, videos, news ─────────────────────────────────────────────────

// ImageResult is one Google Images result.
type ImageResult struct {
	Position     int    `json:"position"`
	Title        string `json:"title"`
	ImageURL     string `json:"image_url"`
	ImageWidth   int    `json:"image_width,omitempty"`
	ImageHeight  int    `json:"image_height,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Source       string `json:"source,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Link         string `json:"link,omitempty"`
}

// ImagesResponse is returned by [Client.Images].
type ImagesResponse struct {
	Request RequestEcho   `json:"request"`
	Results []ImageResult `json:"results"`
	Meta    Meta          `json:"meta"`
}

// VideoResult is one Google Videos result.
type VideoResult struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Domain   string `json:"domain,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Duration string `json:"duration,omitempty"`
	Source   string `json:"source,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Date     string `json:"date,omitempty"`
}

// VideosResponse is returned by [Client.Videos].
type VideosResponse struct {
	Request RequestEcho   `json:"request"`
	Results []VideoResult `json:"results"`
	Meta    Meta          `json:"meta"`
}

// NewsResult is one Google News result (also used for top stories).
type NewsResult struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Domain   string `json:"domain,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Date     string `json:"date,omitempty"`
	Source   string `json:"source,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// NewsResponse is returned by [Client.News].
type NewsResponse struct {
	Request RequestEcho  `json:"request"`
	Results []NewsResult `json:"results"`
	Meta    Meta         `json:"meta"`
}

// ── Places and reviews ───────────────────────────────────────────────────

// PlaceResult is one place (Maps, local results, the search local pack).
type PlaceResult struct {
	Position     int               `json:"position"`
	Title        string            `json:"title"`
	Address      string            `json:"address,omitempty"`
	Latitude     float64           `json:"latitude,omitempty"`
	Longitude    float64           `json:"longitude,omitempty"`
	Rating       float64           `json:"rating,omitempty"`
	RatingCount  int               `json:"rating_count,omitempty"`
	PriceLevel   string            `json:"price_level,omitempty"`
	Type         string            `json:"type,omitempty"`
	Types        []string          `json:"types,omitempty"`
	Website      string            `json:"website,omitempty"`
	PhoneNumber  string            `json:"phone_number,omitempty"`
	OpeningHours map[string]string `json:"opening_hours,omitempty"`
	ThumbnailURL string            `json:"thumbnail_url,omitempty"`
	PlaceID      string            `json:"place_id,omitempty"`
	CID          string            `json:"cid,omitempty"`
	FID          string            `json:"fid,omitempty"`
}

// PlacesResponse is returned by [Client.Maps] and [Client.Places].
type PlacesResponse struct {
	Request RequestEcho   `json:"request"`
	Results []PlaceResult `json:"results"`
	Meta    Meta          `json:"meta"`
}

// ReviewUser is the author of a review.
type ReviewUser struct {
	Name      string `json:"name,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Reviews   int    `json:"reviews,omitempty"`
}

// ReviewReply is the owner's response to a review.
type ReviewReply struct {
	Snippet string `json:"snippet,omitempty"`
	Date    string `json:"date,omitempty"`
}

// ReviewResult is one review.
type ReviewResult struct {
	Rating   float64      `json:"rating,omitempty"`
	Date     string       `json:"date,omitempty"`
	ISODate  string       `json:"iso_date,omitempty"`
	Snippet  string       `json:"snippet,omitempty"`
	Likes    int          `json:"likes,omitempty"`
	User     *ReviewUser  `json:"user,omitempty"`
	Response *ReviewReply `json:"response,omitempty"`
}

// ReviewsResponse is returned by [Client.Reviews].
type ReviewsResponse struct {
	Request RequestEcho    `json:"request"`
	Results []ReviewResult `json:"results"`
	// NextPageToken goes into ReviewsParams.PageToken for the next page; empty on the last page.
	NextPageToken string `json:"next_page_token,omitempty"`
	Meta          Meta   `json:"meta"`
}

// ── Shopping, scholar, patents ───────────────────────────────────────────

// ShoppingResult is one Google Shopping product.
type ShoppingResult struct {
	Position    int     `json:"position"`
	Title       string  `json:"title"`
	Source      string  `json:"source,omitempty"`
	Link        string  `json:"link,omitempty"`
	Price       string  `json:"price,omitempty"`
	PriceValue  float64 `json:"price_value,omitempty"`
	Currency    string  `json:"currency,omitempty"`
	Delivery    string  `json:"delivery,omitempty"`
	ImageURL    string  `json:"image_url,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	RatingCount int     `json:"rating_count,omitempty"`
	Offers      string  `json:"offers,omitempty"`
	ProductID   string  `json:"product_id,omitempty"`
}

// ShoppingResponse is returned by [Client.Shopping].
type ShoppingResponse struct {
	Request RequestEcho      `json:"request"`
	Results []ShoppingResult `json:"results"`
	Meta    Meta             `json:"meta"`
}

// ScholarResult is one Google Scholar paper.
type ScholarResult struct {
	Position        int    `json:"position"`
	Title           string `json:"title"`
	Link            string `json:"link"`
	Domain          string `json:"domain,omitempty"`
	PublicationInfo string `json:"publication_info,omitempty"`
	Snippet         string `json:"snippet,omitempty"`
	Year            int    `json:"year,omitempty"`
	CitedBy         int    `json:"cited_by,omitempty"`
	PDFURL          string `json:"pdf_url,omitempty"`
	ID              string `json:"id,omitempty"`
}

// ScholarResponse is returned by [Client.Scholar].
type ScholarResponse struct {
	Request RequestEcho     `json:"request"`
	Results []ScholarResult `json:"results"`
	Meta    Meta            `json:"meta"`
}

// PatentResult is one Google Patents result.
type PatentResult struct {
	Position          int    `json:"position"`
	Title             string `json:"title"`
	Snippet           string `json:"snippet,omitempty"`
	Link              string `json:"link"`
	PublicationNumber string `json:"publication_number,omitempty"`
	PriorityDate      string `json:"priority_date,omitempty"`
	FilingDate        string `json:"filing_date,omitempty"`
	GrantDate         string `json:"grant_date,omitempty"`
	PublicationDate   string `json:"publication_date,omitempty"`
	Inventor          string `json:"inventor,omitempty"`
	Assignee          string `json:"assignee,omitempty"`
	Language          string `json:"language,omitempty"`
	PDFURL            string `json:"pdf_url,omitempty"`
	ThumbnailURL      string `json:"thumbnail_url,omitempty"`
}

// PatentsResponse is returned by [Client.Patents].
type PatentsResponse struct {
	Request RequestEcho    `json:"request"`
	Results []PatentResult `json:"results"`
	Meta    Meta           `json:"meta"`
}

// ── Autocomplete, webpage ──────────────────────────────────────────

// Suggestion is one autocomplete suggestion.
type Suggestion struct {
	Value string `json:"value"`
}

// AutocompleteResponse is returned by [Client.Autocomplete].
type AutocompleteResponse struct {
	Request RequestEcho  `json:"request"`
	Results []Suggestion `json:"results"`
	Meta    Meta         `json:"meta"`
}

// PageMetadata describes a fetched page.
type PageMetadata struct {
	Title         string `json:"title,omitempty"`
	Description   string `json:"description,omitempty"`
	Language      string `json:"language,omitempty"`
	Canonical     string `json:"canonical,omitempty"`
	SiteName      string `json:"site_name,omitempty"`
	Image         string `json:"image,omitempty"`
	PublishedTime string `json:"published_time,omitempty"`
	Author        string `json:"author,omitempty"`
}

// WebpageResponse is returned by [Client.Webpage].
type WebpageResponse struct {
	Request RequestEcho `json:"request"`
	// URL is the final URL after redirects.
	URL        string       `json:"url"`
	StatusCode int          `json:"status_code,omitempty"`
	Markdown   string       `json:"markdown"`
	Text       string       `json:"text,omitempty"`
	HTML       string       `json:"html,omitempty"`
	Metadata   PageMetadata `json:"metadata"`
	Meta       Meta         `json:"meta"`
}

// ── Rank and account ─────────────────────────────────────────────────────

// RankMatch is one organic result on the checked domain.
type RankMatch struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Link     string `json:"link"`
}

// RankResponse is returned by [Client.Rank].
type RankResponse struct {
	Request RequestEcho `json:"request"`
	Domain  string      `json:"domain"`
	// Position is the best organic position, nil when the domain is not in the checked results.
	Position *int        `json:"position"`
	Matches  []RankMatch `json:"matches"`
	// Checked is the number of organic results inspected.
	Checked int  `json:"checked"`
	Meta    Meta `json:"meta"`
}

// AccountKey describes the calling API key.
type AccountKey struct {
	ID               string   `json:"id,omitempty"`
	Name             string   `json:"name,omitempty"`
	CreditLimit      *float64 `json:"credit_limit,omitempty"`
	CreditsUsedMonth float64  `json:"credits_used_month,omitempty"`
}

// AccountMonth is this month's usage.
type AccountMonth struct {
	Credits  float64 `json:"credits"`
	Requests int     `json:"requests"`
}

// Account is returned by [Client.Account].
type Account struct {
	Balance      float64 `json:"balance"`
	RateLimitRPS int     `json:"rate_limit_rps"`
	// Plan is "free" or "paid".
	Plan            string       `json:"plan"`
	Key             *AccountKey  `json:"key,omitempty"`
	MonthlySpendCap *float64     `json:"monthly_spend_cap,omitempty"`
	Month           AccountMonth `json:"month"`
}

// ── Batches ──────────────────────────────────────────────────────────────

// Batch statuses.
const (
	BatchQueued  = "queued"
	BatchRunning = "running"
	BatchDone    = "done"
	BatchFailed  = "failed"
)

// BatchCreateParams is the body of [BatchesService.Create].
type BatchCreateParams struct {
	Endpoint Endpoint `json:"endpoint"`
	// Requests holds 1-100 request bodies for Endpoint: SearchParams,
	// ReviewsParams, WebpageParams or map[string]any.
	Requests []any `json:"requests"`
	// WebhookURL receives each job's result (signed with X-SerpKite-Signature).
	// Defaults to the account webhook.
	WebhookURL string `json:"webhook_url,omitempty"`
}

// BatchError is why a job failed.
type BatchError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Batch is one queued job.
type Batch struct {
	ID string `json:"id"`
	// Status is queued, running, done or failed.
	Status string `json:"status"`
	// Endpoint is the path, e.g. "/v1/search".
	Endpoint    string     `json:"endpoint"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreditsUsed float64    `json:"credits_used,omitempty"`
	PollURL     string     `json:"poll_url"`
	WebhookURL  *string    `json:"webhook_url,omitempty"`
	// WebhookStatus is pending, delivered, failed or nil.
	WebhookStatus *string     `json:"webhook_status,omitempty"`
	Error         *BatchError `json:"error,omitempty"`
	// Result is the endpoint's response body once done (kept 24 h). Decode it
	// with [Batch.DecodeResult].
	Result json.RawMessage `json:"result,omitempty"`
}

// Finished reports whether the job is done or failed.
func (b *Batch) Finished() bool { return b.Status == BatchDone || b.Status == BatchFailed }

// DecodeResult unmarshals Result into v, e.g. a *SearchResponse. It returns
// false when there is no result yet.
func (b *Batch) DecodeResult(v any) (bool, error) {
	if len(b.Result) == 0 || string(b.Result) == "null" {
		return false, nil
	}
	return true, json.Unmarshal(b.Result, v)
}

// BatchEntry is one entry of [BatchCreateResponse]: exactly one of Batch
// (queued) and Err (rejected at submit time) is set.
type BatchEntry struct {
	Batch *Batch
	Err   *Error
}

// UnmarshalJSON decodes a job or an {"error":{…}} object.
func (e *BatchEntry) UnmarshalJSON(b []byte) error {
	var probe struct {
		ID    string          `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	if probe.ID == "" && len(probe.Error) > 0 && string(probe.Error) != "null" {
		var env errorEnvelope
		if err := json.Unmarshal(b, &env); err != nil {
			return err
		}
		e.Err = &Error{Code: env.Error.Code, Message: env.Error.Message, RequestID: env.Error.RequestID}
		return nil
	}
	e.Batch = new(Batch)
	return json.Unmarshal(b, e.Batch)
}

// MarshalJSON encodes the entry as the API does.
func (e BatchEntry) MarshalJSON() ([]byte, error) {
	if e.Err != nil {
		var env errorEnvelope
		env.Error.Code, env.Error.Message, env.Error.RequestID = e.Err.Code, e.Err.Message, e.Err.RequestID
		return json.Marshal(env)
	}
	return json.Marshal(e.Batch)
}

// BatchCreateResponse is returned by [BatchesService.Create]: one entry per
// request, in order.
type BatchCreateResponse struct {
	Batches []BatchEntry `json:"batches"`
}

// Queued returns the jobs that were accepted, in order.
func (r *BatchCreateResponse) Queued() []*Batch {
	out := make([]*Batch, 0, len(r.Batches))
	for _, e := range r.Batches {
		if e.Batch != nil {
			out = append(out, e.Batch)
		}
	}
	return out
}

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}
