package serpkite

import (
	"cmp"
	"context"
	"net/http"
	"time"
)

// Map returns the URLs of a site: from robots.txt sitemaps (XML, gzip,
// RSS/Atom, plain text; sitemap indexes followed) and the start page's
// links, cleaned (fragments and tracking parameters dropped), kept on the
// site, filtered by IncludePaths / ExcludePaths and de-duplicated. Search
// ranks them by relevance and drops the rest. 1 credit; free when nothing
// is found.
func (c *Client) Map(ctx context.Context, p MapParams, opts ...RequestOption) (*MapResponse, error) {
	return post[MapResponse](ctx, c, "/v1/map", p, opts)
}

// Extract reads up to 20 URLs (HTML or PDF) as Markdown, text or HTML in
// one call, with optional query-ranked highlights (BM25), links and
// images. Each URL that comes back costs 1 credit (0.5 from cache); URLs
// that fail are listed in [ExtractResponse].Failed and cost nothing.
// Summary adds an AI summary per page and OutputSchema (guided by Prompt)
// returns each page's data as JSON, each at an extra per-page price; a page
// whose model step failed carries SummaryError / OutputError instead and is
// not charged for it.
//
// Extract is billed per page even when the response is lost, so it is
// retried only on 429, and its per-attempt timeout covers the server's
// deadline (Timeout, default 50 s, 75 s with Summary or OutputSchema) plus 15 s.
func (c *Client) Extract(ctx context.Context, p ExtractParams, opts ...RequestOption) (*ExtractResponse, error) {
	var out ExtractResponse
	def := extractDefaultTimeout
	if p.Summary || p.OutputSchema != nil {
		def = extractAIDefaultTimeout
	}
	wait := time.Duration(cmp.Or(p.Timeout, def)+15) * time.Second
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: "/v1/extract", body: p, opts: opts, out: &out, rateLimitOnly: true, minTimeout: wait}); err != nil {
		return nil, err
	}
	return &out, nil
}

// extractDefaultTimeout is the server's extract deadline (seconds) when
// Timeout is 0; extractAIDefaultTimeout the one with Summary or OutputSchema.
const (
	extractDefaultTimeout   = 50
	extractAIDefaultTimeout = 75
)
