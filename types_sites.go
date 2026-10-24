package serpkite

import "encoding/json"

// ── Map ──────────────────────────────────────────────────────────────────

// Sitemap modes of [MapParams].Sitemap.
const (
	SitemapInclude = "include" // sitemaps and the start page's links (default)
	SitemapOnly    = "only"    // sitemaps only
	SitemapSkip    = "skip"    // the start page's links only
)

// MapParams are the parameters of [Client.Map].
type MapParams struct {
	// URL is any page of the site (required).
	URL string `json:"url"`
	// Search keeps only URLs relevant to these words, most relevant first.
	// With a thin sitemap, map also reads up to 8 linked pages one level deeper.
	Search string `json:"search,omitempty"`
	// Limit is 1-5000 (default 100).
	Limit             int  `json:"limit,omitempty"`
	IncludeSubdomains bool `json:"include_subdomains,omitempty"`
	// IncludePaths and ExcludePaths are regular expressions (at most 20 each)
	// matched against the URL path: keep only URLs matching one of
	// IncludePaths, drop URLs matching one of ExcludePaths.
	IncludePaths []string `json:"include_paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
	// IgnoreQueryParameters treats URLs that differ only in their query string
	// as one (the first one found is kept).
	IgnoreQueryParameters bool `json:"ignore_query_parameters,omitempty"`
	// Sitemap is [SitemapInclude] (default), [SitemapOnly] or [SitemapSkip].
	Sitemap string `json:"sitemap,omitempty"`
}

// MapURL is one URL of a site map.
type MapURL struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
	// LastMod is the sitemap's lastmod, as written.
	LastMod string `json:"lastmod,omitempty"`
	// Source is "sitemap" or "page".
	Source string `json:"source"`
}

// MapRequestEcho is the normalised request of a map response.
type MapRequestEcho struct {
	Endpoint              string   `json:"endpoint,omitempty"`
	URL                   string   `json:"url,omitempty"`
	Search                string   `json:"search,omitempty"`
	Limit                 int      `json:"limit,omitempty"`
	Sitemap               string   `json:"sitemap,omitempty"`
	IncludeSubdomains     bool     `json:"include_subdomains,omitempty"`
	IncludePaths          []string `json:"include_paths,omitempty"`
	ExcludePaths          []string `json:"exclude_paths,omitempty"`
	IgnoreQueryParameters bool     `json:"ignore_query_parameters,omitempty"`
}

// MapMeta is the meta of a map response.
type MapMeta struct {
	RequestID   string  `json:"request_id"`
	CreditsUsed float64 `json:"credits_used"`
	LatencyMs   int     `json:"latency_ms,omitempty"`
	Count       int     `json:"count"`
}

// MapResponse is returned by [Client.Map].
type MapResponse struct {
	Request MapRequestEcho `json:"request"`
	Results []MapURL       `json:"results"`
	Meta    MapMeta        `json:"meta"`
}

// ── Extract ──────────────────────────────────────────────────────────────

// ExtractParams are the parameters of [Client.Extract].
type ExtractParams struct {
	// URLs are 1-20 HTML pages or PDFs (required; duplicates are dropped).
	URLs []string `json:"urls"`
	// Format is "markdown" (default), "text" or "html".
	Format string `json:"format,omitempty"`
	// Query is what Highlights are ranked against.
	Query string `json:"query,omitempty"`
	// Highlights is the number of query-ranked passages per page (0-10; needs Query).
	Highlights int `json:"highlights,omitempty"`
	// MaxTokens trims each page's markdown or text (100-100000).
	MaxTokens     int  `json:"max_tokens,omitempty"`
	IncludeLinks  bool `json:"include_links,omitempty"`
	IncludeImages bool `json:"include_images,omitempty"`
	// MaxAge accepts a cached page up to this many seconds old (0.5 credit).
	MaxAge int `json:"max_age,omitempty"`
	// Country fetches through an exit in this country (ISO 3166-1 alpha-2).
	Country string `json:"country,omitempty"`
	// Timeout (seconds, 1-90; 0 = the server default: 50, or 75 with Summary or
	// OutputSchema) reports pages still loading as failed (upstream_timeout, not charged).
	Timeout int `json:"timeout,omitempty"`
	// Summary adds an AI summary of each page ([ExtractResult].Summary), focused
	// on Query when set.
	Summary bool `json:"summary,omitempty"`
	// OutputSchema is a JSON Schema (at most 16 KB, e.g. a map or a
	// json.RawMessage): each page's data comes back as JSON matching it in
	// [ExtractResult].Output.
	OutputSchema any `json:"output_schema,omitempty"`
	// Prompt guides OutputSchema extraction (what to pick, how to normalise).
	Prompt string `json:"prompt,omitempty"`
}

// ExtractResult is one page of an extract response.
type ExtractResult struct {
	// URL is the final URL after redirects.
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Title      string `json:"title,omitempty"`
	// PublishedAt is the page's published time as ISO 8601, when it has one.
	PublishedAt string        `json:"published_at,omitempty"`
	Metadata    *PageMetadata `json:"metadata,omitempty"`
	Cached      bool          `json:"cached"`
	Markdown    string        `json:"markdown,omitempty"`
	Text        string        `json:"text,omitempty"`
	// HTML is set with Format "html" (nil for a PDF).
	HTML       *string     `json:"html,omitempty"`
	Links      []PageLink  `json:"links,omitempty"`
	Images     []string    `json:"images,omitempty"`
	Highlights []Highlight `json:"highlights,omitempty"`
	// Summary is set with [ExtractParams].Summary (nil when it failed: SummaryError).
	Summary      *string `json:"summary,omitempty"`
	SummaryError string  `json:"summary_error,omitempty"`
	// Output is the JSON value matching [ExtractParams].OutputSchema (null when
	// it failed: OutputError). Decode it with DecodeOutput.
	Output      json.RawMessage `json:"output,omitempty"`
	OutputError string          `json:"output_error,omitempty"`
}

// DecodeOutput unmarshals Output into v. It returns false when there is no output.
func (r *ExtractResult) DecodeOutput(v any) (bool, error) {
	return decodeRaw(r.Output, v)
}

// ExtractFailure is a URL that could not be read (not charged).
type ExtractFailure struct {
	URL   string             `json:"url"`
	Error ExtractFailureCode `json:"error"`
}

// ExtractFailureCode says why a URL failed: code is invalid_request,
// upstream_error, upstream_timeout, empty_page…
type ExtractFailureCode struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ExtractRequestEcho is the normalised request of an extract response.
type ExtractRequestEcho struct {
	Endpoint string   `json:"endpoint,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Format   string   `json:"format,omitempty"`
	Query    string   `json:"query,omitempty"`
}

// ExtractMeta is the meta of an extract response.
type ExtractMeta struct {
	RequestID   string  `json:"request_id"`
	CreditsUsed float64 `json:"credits_used"`
	LatencyMs   int     `json:"latency_ms,omitempty"`
	Succeeded   int     `json:"succeeded"`
	Failed      int     `json:"failed"`
	// LLM is the model behind Summary / OutputSchema.
	LLM *LLMUsage `json:"llm,omitempty"`
}

// ExtractResponse is returned by [Client.Extract].
type ExtractResponse struct {
	Request ExtractRequestEcho `json:"request"`
	Results []ExtractResult    `json:"results"`
	Failed  []ExtractFailure   `json:"failed"`
	Meta    ExtractMeta        `json:"meta"`
}
